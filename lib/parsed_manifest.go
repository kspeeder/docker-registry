package lib

import (
	"encoding/json"
	"errors"
)

type parsedLayer interface {
	blobSum() string
}

type parsedManifest interface {
	layers() []parsedLayer
}

type parsedLayerV1 struct {
	BlobSum string `json:"blobSum"`
}

type parsedManifestV1 struct {
	Layers []parsedLayerV1 `json:"fsLayers"`
}

type parsedLayerV2 struct {
	Digest string `json:"digest"`
}

/*
{
   "mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
   "schemaVersion": 2,
   "manifests": [
      {
         "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
         "digest": "sha256:708c68409758740d8bba16e0745d25de3df205646f2007055531ee97bd57e885",
         "size": 952,
         "platform": {
            "architecture": "amd64",
            "os": "linux"
         }
      },
      {
         "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
         "digest": "sha256:750618ec58d789688900893168b936f51b0f7f0215b5a0a0f77ea75101abcadf",
         "size": 952,
         "platform": {
            "architecture": "arm",
            "os": "linux",
            "variant": "v6"
         }
      },
      {
         "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
         "digest": "sha256:10255d7e4ad578c50801641807551875feff97feaf9a14fb0e421774901b5a51",
         "size": 952,
         "platform": {
            "architecture": "arm",
            "os": "linux",
            "variant": "v7"
         }
      },
      {
         "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
         "digest": "sha256:a5b42629beaf999ba79afdc95eca9cc008b0a3db73656453d6d1e20071b83886",
         "size": 952,
         "platform": {
            "architecture": "arm64",
            "os": "linux"
         }
      }
   ]
}

{
   "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
   "schemaVersion": 2,
   "config": {
      "mediaType": "application/vnd.docker.container.image.v1+json",
      "digest": "sha256:5aad81aca13912be58110c762d4379ba7a6d4d5da095c4383130f413a6df975b",
      "size": 2670
   },
   "layers": [
      {
         "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
         "digest": "sha256:43c4264eed91be63b206e17d93e75256a6097070ce643c5e8f0379998b44f170",
         "size": 3623807
      },
      {
         "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
         "digest": "sha256:09ed98d9960e01178c73959275b31e319c754d44a98936223206c68262ac581b",
         "size": 69577508
      },
      {
         "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
         "digest": "sha256:1da055758641368ea5c12001b62c4dc8330190cd498ed1715a1405060b871f63",
         "size": 16902488
      }
   ]
}
*/

type parsedManifestV2 struct {
	Layers []parsedLayerV2 `json:"layers"`
}

func (p *parsedLayerV1) blobSum() string {
	return p.BlobSum
}

func (m *parsedManifestV1) layers() (layers []parsedLayer) {
	layers = make([]parsedLayer, 0, len(m.Layers))

	for i := 0; i < len(m.Layers); i++ {
		layers = append(layers, &m.Layers[i])
	}

	return
}

func (p *parsedLayerV2) blobSum() string {
	return p.Digest
}

func (m *parsedManifestV2) layers() (layers []parsedLayer) {
	layers = make([]parsedLayer, 0, len(m.Layers))

	for i := 0; i < len(m.Layers); i++ {
		layers = append(layers, &m.Layers[i])
	}

	return
}

func parseManifest(data []byte) (manifest parsedManifest, err error) {
	var id struct {
		SchemaVersion int `json:"schemaVersion"`
	}

	err = json.Unmarshal(data, &id)
	if err != nil {
		return
	}

	switch id.SchemaVersion {
	case 1:
		var parsedData parsedManifestV1
		err = json.Unmarshal(data, &parsedData)
		manifest = &parsedData

	case 2:
		var parsedData parsedManifestV2
		err = json.Unmarshal(data, &parsedData)
		manifest = &parsedData

	default:
		err = errors.New("unknown manifest schema version")
	}

	return
}
