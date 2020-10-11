package core

import (
    "encoding/json"
    "errors"
    "fmt"
    "io/ioutil"
    "os"
    "path/filepath"
    "strings"

    "github.com/docker/distribution/manifest/manifestlist"
    "github.com/docker/distribution/manifest/schema2"
)


func (i *Image) pull(directory string) error {
    if err := i.auth("pull"); err != nil {
        return err
    }
    url := fmt.Sprintf("%s://%s/v2/%s/%s/manifests/%s", i.Scheme, i.Registry, i.Repo, i.Name, i.Tag)
    resp, err := i.Client.
        R().
        SetHeader("Authorization", i.Authorization()).
        SetHeader("Accept", strings.Join(acceptHeaders, "; ")).
        SetHeader("Accept-Encoding", "gzip").
        SetHeader("User-Agent", "docker/19.03.12 go/go1.13.10 git-commit/48a66213fe kernel/4.19.76-linuxkit os/linux arch/amd64 UpstreamClient(Docker-Client/19.03.12 \\(darwin\\))").
        Get(url)
    if err != nil {
        return err
    }
    imageId := ""
    if resp.StatusCode() != 200 {
        return errors.New("request manifest error")
    }
    typeHeader := resp.Header().Get("Content-Type")
    switch typeHeader {
    case "application/vnd.docker.distribution.manifest.v1+prettyjws":
        fallthrough
    case "application/vnd.docker.distribution.manifest.v1+json":
        var manifest ManifestV1
        if err := json.Unmarshal(resp.Body(), &manifest); err != nil {
            return err
        }
        imageId = manifest.History[0].Id
        for index := range manifest.FSLayers {
            imageJson := manifest.History[index]
            var layer ManifestLayer
            if err := json.Unmarshal([]byte(imageJson.V1Compatibility), &layer); err != nil {
                return err
            }
            layerId := layer.Id
            imageLayer := manifest.FSLayers[index].BlobSum
            layerPath := filepath.Join(directory, i.Registry, i.Repo, i.Name, layerId)
            if _, err := os.Stat(layerPath); os.IsNotExist(err) {
                if err := os.MkdirAll(layerPath, 0700); err != nil {
                    return err
                }
            }
            if err := ioutil.WriteFile(filepath.Join(layerPath, "VERSION"), []byte("1.0"), 0755); err != nil {
                return err
            }
            if err := ioutil.WriteFile(filepath.Join(layerPath, "json"), []byte(imageJson.V1Compatibility), 0755); err != nil {
                return err
            }
            if err := i.fetchBlob(imageLayer, filepath.Join(layerPath, "layer.tar")); err != nil {
                return err
            }
            imageId = layerId
        }
        break
    case "application/vnd.docker.distribution.manifest.v2+json":
        var manifest schema2.Manifest
        if err := json.Unmarshal(resp.Body(), &manifest); err != nil {
            return err
        }
        if err := i.handleManifestV2(&manifest, directory); err != nil {
            return err
        }
        break
    case "application/vnd.docker.distribution.manifest.list.v2+json":
        var manifestList manifestlist.ManifestList
        if err := json.Unmarshal(resp.Body(), &manifestList); err != nil {
            return err
        }
        var digest string
        for _, m := range manifestList.Manifests {
            if m.Platform.Architecture == targetArch && m.Platform.OS == targetOS {
                digest = m.Digest.String()
                break
            }
        }
        if digest == "" {
            return errors.New("no match platform image digest found")
        }
        manifest, err := i.fetchManifestV2(digest)
        if err != nil {
            return err
        }
        if err := i.handleManifestV2(manifest, directory); err != nil {
            return err
        }
        break
    default:
        return fmt.Errorf("unsupported ContentType %s", typeHeader)
    }
    repositoriesBytes := []byte(fmt.Sprintf("{\n\"%s\": { \"%s\": \"%s\" }\n}", i.Name, i.Tag, imageId))
    repositoriesPath := filepath.Join(directory, i.Registry, i.Repo, i.Name, "repositories")
    return ioutil.WriteFile(repositoriesPath, repositoriesBytes, 0644)
}

func (i *Image) fetchManifestV2(digest string) (*schema2.Manifest, error) {
    url := fmt.Sprintf("%s://%s/v2/%s/%s/manifests/%s", i.Scheme, i.Registry, i.Repo, i.Name, digest)
    resp, err := i.Client.
        R().
        SetHeader("Authorization", fmt.Sprintf("Bearer %s", i.AuthInfo.Token)).
        SetHeader("Accept", "application/vnd.docker.distribution.manifest.v2+json").
        Get(url)
    if err != nil {
        return nil, err
    }
    var manifest schema2.Manifest
    if err := json.Unmarshal(resp.Body(), &manifest); err != nil {
        return nil, err
    }
    return &manifest, err
}

func (i *Image) handleManifestV2(manifest *schema2.Manifest, directory string) error {
    var layers []string
    configDigest := manifest.Config.Digest
    imageId := strings.TrimPrefix(configDigest.Encoded(), "sha256:")
    imageConfigPath := filepath.Join(directory, i.Registry, i.Repo, i.Name, fmt.Sprintf("%s.json", imageId))
    if err := i.fetchBlob(configDigest.String(), imageConfigPath); err != nil {
        return err
    }
    parentId := ""
    originParentId := ""
    for index := range manifest.Layers {
        layer := manifest.Layers[index]
        layerMediaType := layer.MediaType
        layerDigest := layer.Digest
        layerId := hashSha256(fmt.Sprintf(`%s\n%s\n`, parentId, layerDigest))
        layerDir := filepath.Join(directory, i.Registry, i.Repo, i.Name, layerId)
        if _, err := os.Stat(layerDir); os.IsNotExist(err) {
            if err := os.MkdirAll(layerDir, 0700); err != nil {
                return err
            }
        }
        if err := ioutil.WriteFile(filepath.Join(layerDir, "VERSION"), []byte("1.0"), 0755); err != nil {
            return err
        }
        layerJsonPath := filepath.Join(layerDir, "json")
        if _, err := os.Stat(layerJsonPath); os.IsNotExist(err) {
            f, err := os.Create(layerJsonPath)
            if err != nil {
                return err
            }
            if err := layerJsonTemplate(layerId, parentId, f); err != nil {
                _ = f.Close()
                return err
            }
            _ = f.Close()
        }
        switch layerMediaType {
        case "application/vnd.docker.image.rootfs.diff.tar.gzip":
            layerTar := filepath.Join(layerDir, "layer.tar")
            if _, err := os.Stat(layerTar); os.IsNotExist(err) {
                if err := i.fetchBlob(layerDigest.String(), layerTar); err != nil {
                    return err
                }
            }
            layers = append(layers, filepath.Join(layerId, "layer.tar"))
        }
        originParentId = parentId
        parentId = layerId
    }
    lastLayerId := parentId
    parentId = originParentId
    imageConfigBytes, err := ioutil.ReadFile(imageConfigPath)
    if err != nil {
        return err
    }
    var imageConfigJson map[string]interface{}
    if err := json.Unmarshal(imageConfigBytes, &imageConfigJson); err != nil {
        return err
    }
    imageConfigJson["id"] = lastLayerId
    if parentId != "" {
        imageConfigJson["parentId"] = parentId
    }
    lastLayerJsonBytes, err := json.Marshal(imageConfigJson)
    if err != nil {
        return err
    }
    lastLayerPath := filepath.Join(directory, i.Registry, i.Repo, i.Name, lastLayerId, "json")
    if err := ioutil.WriteFile(lastLayerPath, lastLayerJsonBytes, 0644); err != nil {
        return err
    }
    var imageManifest []LocalManifest
    imageManifest = append(imageManifest, LocalManifest{
        Config: filepath.Join(lastLayerId, "json"),
        RepoTags: []string{fmt.Sprintf("%s:%s", i.Name, i.Tag)},
        Layers: layers,
    })
    imageManifestBytes, err := json.Marshal(imageManifest)
    if err != nil {
        return err
    }
    imageManifestPath := filepath.Join(directory, i.Registry, i.Repo, i.Name, "manifest.json")
    return ioutil.WriteFile(imageManifestPath, imageManifestBytes, 0644)
}