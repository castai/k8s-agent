module castai-agent

go 1.25.0

require (
	cloud.google.com/go/compute/metadata v0.7.0
	github.com/DataDog/extendeddaemonset/api v0.0.0-20250630134016-9c1848fbb2b1
	github.com/KimMachineGun/automemlimit v0.7.3
	github.com/argoproj/argo-rollouts v1.8.3
	github.com/aws/aws-sdk-go-v2 v1.37.2
	github.com/aws/aws-sdk-go-v2/config v1.30.3
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.2
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.240.0
	github.com/fsnotify/fsnotify v1.9.0
	github.com/go-resty/resty/v2 v2.16.5
	github.com/golang/mock v1.6.0
	github.com/google/gnostic-models v0.7.0
	github.com/google/uuid v1.6.0
	github.com/jarcoal/httpmock v1.4.0
	github.com/prometheus/client_golang v1.22.0
	github.com/samber/lo v1.51.0
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/cobra v1.9.1
	github.com/spf13/pflag v1.0.6
	github.com/spf13/viper v1.20.1
	github.com/stretchr/testify v1.10.0
	go.uber.org/goleak v1.3.0
	golang.org/x/exp v0.0.0-20250620022241-b7579e27df2b
	golang.org/x/net v0.41.0
	golang.org/x/sync v0.15.0
	k8s.io/api v0.33.4
	k8s.io/apiextensions-apiserver v0.33.4
	k8s.io/apimachinery v0.33.4
	k8s.io/client-go v0.33.4
	k8s.io/component-base v0.33.4
	k8s.io/metrics v0.33.4
	k8s.io/utils v0.0.0-20250604170112-4c0f3b243397
	sigs.k8s.io/controller-runtime v0.21.0
)

require (
	github.com/aws/aws-sdk-go-v2/credentials v1.18.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.2 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.2 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.27.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.32.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.36.0 // indirect
	github.com/aws/smithy-go v1.22.5 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/emicklei/go-restful/v3 v3.12.2 // indirect
	github.com/fxamacker/cbor/v2 v2.8.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-openapi/jsonpointer v0.21.1 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.3.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20250630185457-6e76a2b096b5 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.65.0 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/sagikazarmark/locafero v0.9.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.14.0 // indirect
	github.com/spf13/cast v1.9.2 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.opentelemetry.io/otel v1.37.0 // indirect
	go.opentelemetry.io/otel/trace v1.37.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/oauth2 v0.30.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/term v0.32.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	golang.org/x/time v0.12.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20250628140032-d90c4fd18f59 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.7.0 // indirect
	sigs.k8s.io/yaml v1.5.0 // indirect
)

replace github.com/chzyer/logex v1.1.10 => github.com/chzyer/logex v1.2.0

exclude (
	// We require github.com/argoproj/argo-rollouts which depends on k8s.io/kubernetes.
	//
	// However, k8s.io/kubernetes itself requires k8s.io/* modules.
	// Since k8s.io/kubernetes is a monorepo that requires and implements such modules,
	// in its `go.mod` it uses `require k8s.io/sample-module-name v0.0.0` to signal the requirement:
	// https://github.com/kubernetes/kubernetes/blob/948afe5ca072329a73c8e79ed5938717a5cb3d21/go.mod#L88-L118
	// and then later replaces such modules with implementations local to the monorepo:
	// https://github.com/kubernetes/kubernetes/blob/948afe5ca072329a73c8e79ed5938717a5cb3d21/go.mod#L228-L257
	//
	// When we require k8s.io/kubernetes and need to list all required modules,
	// for example, so that the IDE can index the symbols for the project,
	// it runs some variation of the following command:
	//   go list -m -json -mod=mod all
	// Unfortunately, the aforementioned approach makes this command error out,
	// since k8s.io/kubernetes requires modules v0.0.0 which don't exist and aren't published.
	// To sidestep this limitation, we exclude such modules from our dependency graph.
	// Details:
	//  * https://go.dev/doc/modules/gomod-ref#exclude
	//  * https://go.dev/ref/mod#go-mod-file-exclude
	k8s.io/cloud-provider v0.0.0
	k8s.io/kubelet v0.0.0
)
