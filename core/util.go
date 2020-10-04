package core

import (
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "fmt"
    "io"
    "os"
    "regexp"
    "strings"
    "text/template"
)

const (
    //urlRegex        = `[(http(s)?)://(www.)?a-zA-Z0-9@:%._+~#=]{2,256}\.[a-z]{2,6}\b([-a-zA-Z0-9@:%_+.~#?&/=]*)`
    //urlRegex =
    layerJsonGoTmpl = `{
  "id": "{{ .id }}",
  {{- if .parent }}
  "parent": "{{ .parent }}",
  {{- end }}
  "created": "0001-01-01T00:00:00Z",
  "container_config": {
    "Hostname": "",
    "Domainname": "",
    "User": "",
    "AttachStdin": false,
    "AttachStdout": false,
    "AttachStderr": false,
    "Tty": false,
    "OpenStdin": false,
    "StdinOnce": false,
    "Env": null,
    "Cmd": null,
    "Image": "",
    "Volumes": null,
    "WorkingDir": "",
    "Entrypoint": null,
    "OnBuild": null,
    "Labels": null
  }
}`
)

var acceptHeaders = []string{
    "application/vnd.docker.distribution.manifest.v2+json",
    "application/vnd.docker.distribution.manifest.list.v2+json",
    "application/vnd.docker.distribution.manifest.v1+prettyjws",
    "application/json",
}

func parseTag(s string) (string, string, error) {
    colonSplitStr := strings.Split(s, ":")
    switch len(colonSplitStr) {
    case 1:
        return s, "latest", nil
    case 2:
        return colonSplitStr[0], colonSplitStr[1], nil
    default:
        return "", "", errors.New(fmt.Sprintf("error image format: %q", s))
    }
}

func matchAuthUrls(s string) (*RegistryAuth, error) {
    regex, err := regexp.Compile(`"(.*?)"`)
    if err != nil {
        return nil, err
    }
    matchStrings := regex.FindAllString(s, -1)
    if len(matchStrings) != 2 {
        return nil, fmt.Errorf("get realm and service from header %s failed", s)
    }
    return &RegistryAuth{
        Realm:   strings.ReplaceAll(matchStrings[0], "\"", ""),
        Service: strings.ReplaceAll(matchStrings[1], "\"", ""),
        Token:   "",
    }, nil
}

func hashSha256(s string) string {
    h := sha256.New()
    h.Write([]byte(s))
    return hex.EncodeToString(h.Sum(nil))
}

func layerJsonTemplate(id, parentId string, writer io.Writer) error {
    t, err := template.New("layerJson").Parse(layerJsonGoTmpl)
    if err != nil {
        return err
    }
    return t.Execute(writer, map[string]string{
        "id":     id,
        "parent": parentId,
    })
}

func getFileSize(file string) int64 {
    f, _ := os.Open(file)
    stat, _ := f.Stat()
    defer func() {
        _ = f.Close()
    }()
    return stat.Size()
}
