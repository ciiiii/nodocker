package core

import (
    "encoding/json"
    "fmt"
)

type RegistryAuth struct {
    Realm   string
    Service string
    Token   string
}

type RegistryAccount struct {
    Username string
    Password string
}

func (i *Image) prepareAuth() error {
    resp, err := i.Client.R().Get(fmt.Sprintf("%s://%s/v2/", i.Scheme, i.Registry))
    if err != nil {
        return err
    }
    headers := resp.Header()
    authHeader := headers.Get("Www-Authenticate")
    registryAuth, err := matchAuthUrls(authHeader)
    if err != nil {
        return err
    }
    i.AuthInfo = *registryAuth
    return nil
}

func (i *Image) auth(operation string) error {
    if i.AuthInfo.Realm == "" {
        return nil
    }
    req := i.Client.R().
        SetQueryParam("service", i.AuthInfo.Service).
        SetQueryParam("scope", fmt.Sprintf("repository:%s/%s:%s", i.Repo, i.Name, operation))
    if i.Account != nil {
        req = req.
            SetQueryParam("account", i.Account.Username).
            SetBasicAuth(i.Account.Username, i.Account.Password)
    }
    resp, err := req.Get(i.AuthInfo.Realm)
    if err != nil {
        return err
    }
    var content struct {
        Token string `json:"token"`
    }
    if err := json.Unmarshal(resp.Body(), &content); err != nil {
        return err
    }
    i.AuthInfo.Token = content.Token
    return nil
}

func (i *Image) Authorization() string  {
    return fmt.Sprintf("Bearer %s", i.AuthInfo.Token)
}