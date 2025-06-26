package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *ServiceImpl) GetKOPSClusterNameAndStateStore(ctx context.Context, log logrus.FieldLogger) (clusterName, stateStore string, reterr error) {
	ns, err := s.getKubeSystemNamespace(ctx)
	if err != nil {
		return "", "", err
	}

	for k, v := range ns.Annotations {
		manifest, ok := kopsAddonAnnotation(log, k, v)
		if !ok {
			continue
		}

		path := manifest.Channel.Path
		if path[0] == '/' {
			path = path[1:]
		}

		name := strings.Split(path, "/")[0]
		store := fmt.Sprintf("%s://%s", manifest.Channel.Scheme, manifest.Channel.Host)

		return name, store, nil
	}

	return "", "", errors.New("failed discovering cluster properties: cluster name, state store")
}

type kopsAddonManifest struct {
	Version      string
	Channel      url.URL
	ID           string
	ManifestHash string
}

func kopsAddonAnnotation(log logrus.FieldLogger, k, v string) (*kopsAddonManifest, bool) {
	if !strings.HasPrefix(k, "addons.k8s.io/") {
		return nil, false
	}

	manifest := map[string]interface{}{}
	if err := json.Unmarshal([]byte(v), &manifest); err != nil {
		log.Debugf("failed unmarshalling %q namespace annotation %q value %s: %v", metav1.NamespaceSystem, k, v, err)
		return nil, false
	}

	channel := manifest["channel"].(string)

	if len(channel) == 0 {
		log.Debugf(`%q namespace annotation %q value %s does not have the "channel" property`, metav1.NamespaceSystem, k, v)
		return nil, false
	}

	uri, err := url.Parse(channel)
	if err != nil {
		log.Debugf("%q namespace annotation %q channel value %s is not a valid uri: %v", metav1.NamespaceSystem, k, channel, err)
		return nil, false
	}

	if len(uri.Scheme) == 0 {
		log.Debugf("%q namespace annotation %q channel scheme %s is empty", metav1.NamespaceSystem, k, channel)
		return nil, false
	}

	if len(uri.Host) == 0 {
		log.Debugf("%q namespace annotation %q channel host %s is empty", metav1.NamespaceSystem, k, channel)
		return nil, false
	}

	if len(uri.Path) == 0 {
		log.Debugf("%q namespace annotation %q channel path %s is empty", metav1.NamespaceSystem, k, channel)
		return nil, false
	}

	var version, id, hash string
	if val, ok := manifest["version"]; ok {
		version = val.(string)
	}
	if val, ok := manifest["id"]; ok {
		id = val.(string)
	}
	if val, ok := manifest["manifestHash"]; ok {
		hash = val.(string)
	}

	return &kopsAddonManifest{
		Channel:      *uri,
		Version:      version,
		ID:           id,
		ManifestHash: hash,
	}, true
}
