package core

import (
    "bytes"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"

    "github.com/docker/distribution/manifest/schema2"
)

const (
    chunkSize = 2097152
)

func (i *Image) fetchBlob(digest, targetFile string) error {
    r, err := i.Client.
        R().
        SetHeader("Authorization", fmt.Sprintf("Bearer %s", i.AuthInfo.Token)).
        SetOutput(targetFile).
        Get(fmt.Sprintf("https://%s/v2/%s/%s/blobs/%s", i.Registry, i.Repo, i.Name, digest))
    if err != nil {
        return err
    }
    if r.StatusCode() != 200 {
        log.Println(r.RawResponse)
        return fmt.Errorf("can't download file from %q", r.Request.URL)
    }
    return nil
}

func (i *Image) uploadBlob(digest, sourceFile string) error {
    url, err := i.prepareUploading()
    if err != nil {
        return err
    }
    f, err := os.Open(sourceFile)
    if err != nil {
        return err
    }
    defer func() {
        _ = f.Close()
    }()
    stat, err := f.Stat()
    if err != nil {
        return err
    }
    fSize := stat.Size()
    start, end := 0, 0
    buf := make([]byte, chunkSize)
    h := sha256.New()
    for {
        n, err := f.Read(buf)
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }
        end = start + n
        log.Printf("Pushing %s ... %.2f%s", digest, (float64(end)/float64(fSize))*100, "%")
        chunk := buf[0:n]
        h.Write(chunk)
        if int64(end) == fSize {
            sum := h.Sum(nil)
            hash := hex.EncodeToString(sum)
            resp, err := i.Client.R().
                SetHeader("Authorization", i.Authorization()).
                SetHeader("Content-Type", "application/octet-stream").
                SetHeader("Content-Length", fmt.Sprintf("%d", n)).
                SetHeader("Content-Range", fmt.Sprintf("%d-%d", stat, end)).
                SetQueryParam("digest", fmt.Sprintf("sha256:%s", hash)).
                SetBody(bytes.NewBuffer(chunk)).
                Put(url)
            if err != nil {
                return err
            }
            if resp.StatusCode() != http.StatusCreated {
                return fmt.Errorf("PUT chunk error\ncode: %d\nbody:%s", resp.StatusCode(), resp.Body())
            }
            break
        } else {
            resp, err := i.Client.R().
                SetHeader("Authorization", i.Authorization()).
                SetHeader("Content-Type", "application/octet-stream").
                SetHeader("Accept-Encoding", "gzip").
                SetHeader("Transfer-Encoding", "chunked").
                SetHeader("Content-Length", fmt.Sprintf("%d", n)).
                SetHeader("Content-Range", fmt.Sprintf("%d-%d", stat, end)).
                SetBody(bytes.NewBuffer(chunk)).
                Patch(url)
            if err != nil {
                return err
            }
            location := resp.Header().Get("Location")
            if resp.StatusCode() == http.StatusAccepted && location != "" {
                url = location
            } else {
                return fmt.Errorf("PATCH chunk error\ncode: %d\nbody:%s", resp.StatusCode(), resp.Body())
            }
            start = end
        }
    }
    return nil
}

func (i *Image) prepareUploading() (string, error)  {
    url := fmt.Sprintf("%s://%s/v2/%s/%s/blobs/uploads/", i.Scheme, i.Registry, i.Repo, i.Name)
    resp, err := i.Client.
        R().
        SetHeader("Authorization", i.Authorization()).
        Post(url)
    if err != nil {
        return "", err
    }
    location := resp.Header().Get("Location")
    if resp.StatusCode() == http.StatusAccepted && location != "" {
        return location, nil
    }
    return "", fmt.Errorf("uploads error\ncode: %d\nbody:%s", resp.StatusCode(), resp.Body())
}

func (i *Image) uploadManifest(manifest schema2.Manifest) error {
    manifestBytes, err := json.Marshal(manifest)
    if err != nil {
        return err
    }
    resp, err := i.Client.R().
        SetHeader("Authorization", i.Authorization()).
        SetHeader("Content-Type", schema2.MediaTypeManifest).
        SetBody(manifestBytes).
        Put(fmt.Sprintf("%s://%s/v2/%s/%s/manifests/%s", i.Scheme, i.Registry, i.Repo, i.Name, i.Tag))
    if err != nil {
        return err
    }
    if resp.StatusCode() != http.StatusCreated {
        return fmt.Errorf("PUT manifest error\ncode: %d\nbody:%s", resp.StatusCode(), resp.Body())
    }
    return nil
}