package core

import (
    "encoding/json"
    "errors"
    "fmt"
    "io/ioutil"
    "log"
    "net/http"
    "path/filepath"

    "github.com/docker/distribution"
    "github.com/docker/distribution/manifest/schema2"
    "github.com/opencontainers/go-digest"
)

func (i *Image) push(directory string) error {
    if err := i.auth("push,pull"); err != nil {
        return err
    }
    resp, err := i.Client.
        R().
        SetHeader("Authorization", i.Authorization()).
        Post(fmt.Sprintf("%s://%s/v2/%s/%s/blobs/uploads/", i.Scheme, i.Registry, i.Repo, i.Name))
    if err != nil {
        return err
    }
    if resp.StatusCode() == http.StatusUnauthorized {
        return errors.New("unauthorized push request")
    }
    manifestFilePath := filepath.Join(directory, "manifest.json")
    manifestFile, err := ioutil.ReadFile(manifestFilePath)
    if err != nil {
        return err
    }
    var manifests []LocalManifest
    if err := json.Unmarshal(manifestFile, &manifests); err != nil {
        return err
    }
    manifest := manifests[0]
    newManifest := schema2.Manifest{}
    for _, layer := range manifest.Layers {
        layerPath := filepath.Join(directory, layer)
        layerFile, err := ioutil.ReadFile(layerPath)
        if err != nil {
            return err
        }
        layerHash := hashSha256(string(layerFile))
        exist, err := i.checkLayerExist(layerHash)
        if err != nil {
            return err
        }
        if exist {
            log.Printf("layer: %s exist", layerHash)
        } else {
            if err := i.uploadBlob(layerHash, layerPath); err != nil {
                return err
            }
        }
        newManifest.Layers = append(newManifest.Layers, distribution.Descriptor{
            MediaType: schema2.MediaTypeLayer,
            Size: getFileSize(layerPath),
            Digest: digest.Digest("sha256:" + layerHash),
        })
    }
    configPath := filepath.Join(directory, manifest.Config)
    configFile, err := ioutil.ReadFile(configPath)
    if err != nil {
        return err
    }
    configHash := hashSha256(string(configFile))
    exist, err := i.checkLayerExist(configHash)
    if err != nil {
        return err
    }
    if exist {
        log.Printf("config: %s exist", configHash)
    } else {
        if err := i.uploadBlob(configHash, configPath); err != nil {
            return err
        }
    }
    newManifest.Config.MediaType = schema2.MediaTypeImageConfig
    newManifest.Config.Size = getFileSize(configPath)
    newManifest.Config.Digest = digest.Digest("sha256:" + configHash)
    newManifest.SchemaVersion = schema2.SchemaVersion.SchemaVersion
    newManifest.MediaType = schema2.SchemaVersion.MediaType
    if err := i.uploadManifest(newManifest); err != nil {
        return err
    }
    return nil
}

func (i *Image) checkLayerExist(layerId string) (bool, error) {
    url := fmt.Sprintf("%s://%s/v2/%s/%s/blobs/sha256:%s/", i.Scheme, i.Registry, i.Repo, i.Name, layerId)
    resp, err := i.Client.R().SetHeader("Authorization", i.Authorization()).Head(url)
    if err != nil {
        return false, err
    }
    return resp.StatusCode() != http.StatusNotFound, nil
}