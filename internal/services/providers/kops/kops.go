package kops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"castai-agent/internal/castai"
	"castai-agent/internal/config"
	awsclient "castai-agent/internal/services/providers/eks/client"
	"castai-agent/internal/services/providers/gke"
	"castai-agent/internal/services/providers/types"
	"castai-agent/pkg/labels"
)

const Name = "kops"

func New(_ context.Context, log logrus.FieldLogger, clientset kubernetes.Interface) (types.Provider, error) {
	return &Provider{
		log:       log,
		clientset: clientset,
	}, nil
}

type Provider struct {
	log       logrus.FieldLogger
	clientset kubernetes.Interface
	awsClient awsclient.Client
	csp       string
}

func (p *Provider) RegisterCluster(ctx context.Context, client castai.Client) (*types.ClusterRegistration, error) {
	ns, err := p.clientset.CoreV1().Namespaces().Get(ctx, metav1.NamespaceSystem, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting namespace %q: %w", metav1.NamespaceSystem, err)
	}

	clusterID, err := getClusterID(ns)
	if err != nil {
		return nil, fmt.Errorf("getting cluster id: %w", err)
	}

	var csp, region, clusterName, stateStore string

	if cfg := config.Get().KOPS; cfg != nil {
		csp, region, clusterName, stateStore = cfg.CSP, cfg.Region, cfg.ClusterName, cfg.StateStore
	} else {
		c, r, err := p.getCSPAndRegion(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("getting csp and region: %w", err)
		}

		n, s, err := p.getClusterNameAndStateStore(ns)
		if err != nil {
			return nil, fmt.Errorf("getting cluster name and state store: %w", err)
		}

		csp, region, clusterName, stateStore = *c, *r, *n, *s
	}

	p.log.Infof(
		"discovered kops cluster properties csp=%s region=%s cluster_name=%s state_store=%s",
		csp,
		region,
		clusterName,
		stateStore,
	)

	p.csp = csp

	if p.csp == "aws" {
		opts := []awsclient.Opt{
			awsclient.WithMetadata("", region, clusterName),
			awsclient.WithEC2Client(),
		}
		c, err := awsclient.New(ctx, p.log, opts...)
		if err != nil {
			p.log.Errorf("failed initializing aws client, spot functionality will be reduced: %v", err)
		} else {
			p.awsClient = c
		}
	}

	resp, err := client.RegisterCluster(ctx, &castai.RegisterClusterRequest{
		ID:   *clusterID,
		Name: clusterName,
		KOPS: &castai.KOPSParams{
			CSP:         csp,
			Region:      region,
			ClusterName: clusterName,
			StateStore:  stateStore,
		},
	})
	if err != nil {
		return nil, err
	}

	return &types.ClusterRegistration{
		ClusterID:      resp.ID,
		OrganizationID: resp.OrganizationID,
	}, nil
}

func (p *Provider) IsSpot(ctx context.Context, node *v1.Node) (bool, error) {
	if val, ok := node.Labels[labels.Spot]; ok && val == "true" {
		return true, nil
	}

	if p.csp == "aws" && p.awsClient != nil {
		hostname, ok := node.Labels[v1.LabelHostname]
		if !ok {
			return false, fmt.Errorf("label %s not found on node %s", v1.LabelHostname, node.Name)
		}

		instances, err := p.awsClient.GetInstancesByPrivateDNS(ctx, []string{hostname})
		if err != nil {
			return false, fmt.Errorf("getting instances by hostname: %w", err)
		}

		for _, instance := range instances {
			if instance.InstanceLifecycle != nil && *instance.InstanceLifecycle == "spot" {
				return true, nil
			}
		}
	}

	if p.csp == "gcp" {
		if val, ok := node.Labels[gke.LabelPreemptible]; ok && val == "true" {
			return true, nil
		}
	}

	return false, nil
}

func (p *Provider) Name() string {
	return Name
}

func (p *Provider) getClusterNameAndStateStore(ns *v1.Namespace) (clusterName, stateStore *string, reterr error) {
	for k, v := range ns.Annotations {
		manifest, ok := kopsAddonAnnotation(p.log, k, v)
		if !ok {
			continue
		}

		path := manifest.Channel.Path
		if path[0] == '/' {
			path = path[1:]
		}

		name := strings.Split(path, "/")[0]
		store := fmt.Sprintf("%s://%s", manifest.Channel.Scheme, manifest.Channel.Host)

		return &name, &store, nil
	}

	return nil, nil, errors.New("failed discovering cluster properties: cluster name, state store")
}

func (p *Provider) getCSPAndRegion(ctx context.Context, next string) (csp, region *string, reterr error) {
	nodes, err := p.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 10, Continue: next})
	if err != nil {
		return nil, nil, fmt.Errorf("listing nodes: %w", err)
	}

	for i, n := range nodes.Items {
		ready := false

		for _, cond := range n.Status.Conditions {
			if cond.Type == v1.NodeReady && cond.Status == v1.ConditionTrue {
				ready = true
				break
			}
		}

		if !ready {
			continue
		}

		nodeCSP, ok := getCSP(&nodes.Items[i])
		if ok {
			csp = &nodeCSP
		}

		nodeRegion, ok := getRegion(&nodes.Items[i])
		if ok {
			region = &nodeRegion
		}

		if csp != nil && region != nil {
			return csp, region, nil
		}
	}

	if nodes.Continue != "" {
		return p.getCSPAndRegion(ctx, nodes.Continue)
	}

	var properties []string
	if csp == nil {
		properties = append(properties, "csp")
	}
	if region == nil {
		properties = append(properties, "region")
	}

	return nil, nil, fmt.Errorf("failed discovering properties: %s", strings.Join(properties, ", "))
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

func getClusterID(ns *v1.Namespace) (*uuid.UUID, error) {
	clusterID, err := uuid.Parse(string(ns.UID))
	if err != nil {
		return nil, fmt.Errorf("parsing namespace %q uid: %w", metav1.NamespaceSystem, err)
	}
	return &clusterID, nil
}

func getRegion(n *v1.Node) (string, bool) {
	if val, ok := n.Labels[v1.LabelTopologyRegion]; ok {
		return val, true
	}

	if val, ok := n.Labels[v1.LabelFailureDomainBetaRegion]; ok {
		return val, true
	}

	return "", false
}

func getCSP(n *v1.Node) (string, bool) {
	providerID := n.Spec.ProviderID

	if strings.HasPrefix(providerID, "gce://") {
		return "gcp", true
	}

	if strings.HasPrefix(providerID, "aws://") {
		return "aws", true
	}

	return "", false
}
