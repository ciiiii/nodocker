package core

type ManifestV1 struct {
    SchemaVersion int    `json:"schemaVersion"`
    Name          string `json:"name"`
    Tag           string `json:"tag"`
    Architecture  string `json:"architecture"`
    FSLayers      []struct {
        BlobSum string `json:"blobSum"`
    } `json:"fsLayers"`
    History []struct {
        Id string `json:"id"`
        V1Compatibility string `json:"v1Compatibility"`
    } `json:"history"`
}

type ManifestLayer struct {
    Id string `json:"id"`
}

type LocalManifest struct {
    Config string `json:"Config"`
    RepoTags []string `json:"repoTags"`
    Layers []string `json:"Layers"`
}
