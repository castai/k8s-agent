module castai-agent

go 1.16

require (
	github.com/aws/aws-sdk-go v1.37.23
	github.com/cenkalti/backoff/v4 v4.1.0
	github.com/go-resty/resty/v2 v2.5.0
	github.com/golang/mock v1.4.1
	github.com/google/uuid v1.1.2
	github.com/jarcoal/httpmock v1.0.8
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.6.1
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	k8s.io/utils v0.0.0-20210111153108-fddb29f9d009
	sigs.k8s.io/controller-runtime v0.8.3
)
