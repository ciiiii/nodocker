package core

import (
    "errors"
    "fmt"
    "path/filepath"
    "strings"

    "github.com/go-resty/resty/v2"
)

const (
    targetArch = "amd64"
    targetOS   = "linux"
)

type Image struct {
    Client   *resty.Client
    Registry string
    Repo     string
    Name     string
    Tag      string
    Scheme   string
    Account  *RegistryAccount
    AuthInfo RegistryAuth
}

func NewImage(s string, insecure bool, account *RegistryAccount) (*Image, error) {
    if strings.HasPrefix(s, "docker.io") {
        s = strings.ReplaceAll(s, "docker.io", "registry-1.docker.io")
    }
    if strings.HasPrefix(s, "k8s.gcr.io") {
        s = strings.ReplaceAll(s, "k8s.gcr.io", "gcr.io/google-containers")
    }

    var image Image
    if insecure {
        image.Scheme = "http"
    }else {
        image.Scheme = "https"
    }
    image.Account = account
    image.Client = resty.New()
    slashSplitStr := strings.Split(s, "/")
    switch len(slashSplitStr) {
    case 1:
        image.Registry = "registry-1.docker.io"
        image.Repo = "library"
        if imageName, tag, err := parseTag(s); err != nil {
            return nil, err
        } else {
            image.Name = imageName
            image.Tag = tag
        }
        return &image, nil
    case 2:
        image.Registry = "registry-1.docker.io"
        image.Repo = slashSplitStr[0]
        if imageName, tag, err := parseTag(slashSplitStr[1]); err != nil {
            return nil, err
        } else {
            image.Name = imageName
            image.Tag = tag
        }
        return &image, nil
    case 3:
        image.Registry = slashSplitStr[0]
        image.Repo = slashSplitStr[1]
        if imageName, tag, err := parseTag(slashSplitStr[2]); err != nil {
            return nil, err
        } else {
            image.Name = imageName
            image.Tag = tag
        }
        return &image, nil
    default:
        return nil, errors.New(fmt.Sprintf("error image format: %q", s))
    }
}

func (i *Image) TargetPath(directory string) string {
    return filepath.Join(directory, i.Registry, i.Repo, i.Name)
}

func (i *Image) Pull(directory string) error {
    if err := i.prepareAuth(); err != nil {
        return err
    }
    if err := i.pull(directory); err != nil {
        return err
    }
    return nil
}

func (i *Image) Push(directory string) error {
    if err := i.prepareAuth(); err != nil {
        return err
    }
    if err := i.push(directory); err != nil {
        return err
    }
    return nil
}
