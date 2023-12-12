module github.com/aws/amazon-cloudwatch-agent-test

go 1.18

// Avoid checksum mismatch for go-collectd https://github.com/collectd/go-collectd/issues/94
replace collectd.org v0.5.0 => github.com/collectd/go-collectd v0.5.0

require (
	collectd.org v0.5.0
	github.com/DataDog/datadog-go v4.8.3+incompatible
	github.com/aws/aws-sdk-go v1.48.12
	github.com/aws/aws-sdk-go-v2 v1.23.5
	github.com/aws/aws-sdk-go-v2/config v1.25.11
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.12.9
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.14.9
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.15.4
	github.com/aws/aws-sdk-go-v2/service/cloudformation v1.42.0
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.31.2
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.29.2
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.26.3
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.138.2
	github.com/aws/aws-sdk-go-v2/service/ecs v1.35.2
	github.com/aws/aws-sdk-go-v2/service/s3 v1.47.2
	github.com/aws/aws-sdk-go-v2/service/ssm v1.44.2
	github.com/aws/aws-sdk-go-v2/service/xray v1.23.2
	github.com/aws/aws-xray-sdk-go v1.8.3
	github.com/cenkalti/backoff/v4 v4.2.1
	github.com/google/uuid v1.4.0
	github.com/mitchellh/mapstructure v1.5.0
	github.com/prozz/aws-embedded-metrics-golang v1.2.0
	github.com/qri-io/jsonschema v0.2.1
	github.com/shirou/gopsutil/v3 v3.23.3
	github.com/stretchr/testify v1.8.4
	go.opentelemetry.io/contrib/propagators/aws v1.21.1
	go.opentelemetry.io/otel v1.21.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.21.0
	go.opentelemetry.io/otel/sdk v1.21.0
	go.opentelemetry.io/otel/trace v1.21.0
	go.uber.org/multierr v1.11.0
	golang.org/x/exp v0.0.0-20231127185646-65229373498e
	golang.org/x/sys v0.15.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/andybalholm/brotli v1.0.6 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.5.3 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.16.9 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.2.8 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.5.8 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.7.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.2.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodbstreams v1.18.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.10.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.2.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.8.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.10.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.16.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.18.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.21.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.26.2 // indirect
	github.com/aws/smithy-go v1.18.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.18.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/compress v1.17.4 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/qri-io/jsonpointer v0.1.1 // indirect
	github.com/shoenig/go-m1cpu v0.1.4 // indirect
	github.com/tklauser/go-sysconf v0.3.11 // indirect
	github.com/tklauser/numcpus v0.6.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.51.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.21.0 // indirect
	go.opentelemetry.io/otel/metric v1.21.0 // indirect
	go.opentelemetry.io/proto/otlp v1.0.0 // indirect
	golang.org/x/mod v0.14.0 // indirect
	golang.org/x/net v0.19.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.16.0 // indirect
	google.golang.org/genproto v0.0.0-20231127180814-3a041ad873d4 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20231127180814-3a041ad873d4 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231127180814-3a041ad873d4 // indirect
	google.golang.org/grpc v1.59.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)
