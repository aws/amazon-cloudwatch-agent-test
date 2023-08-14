module github.com/aws/amazon-cloudwatch-agent-test

go 1.18

// Avoid checksum mismatch for go-collectd https://github.com/collectd/go-collectd/issues/94
replace collectd.org v0.5.0 => github.com/collectd/go-collectd v0.5.0

require (
	collectd.org v0.5.0
	github.com/DataDog/datadog-go v4.8.3+incompatible
	github.com/aws/aws-sdk-go v1.44.262
	github.com/aws/aws-sdk-go-v2 v1.20.1
	github.com/aws/aws-sdk-go-v2/config v1.18.10
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.10.0
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.12.21
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.11.49
	github.com/aws/aws-sdk-go-v2/service/cloudformation v1.27.4
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.25.2
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.15.20
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.18.2
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.77.0
	github.com/aws/aws-sdk-go-v2/service/ecs v1.23.2
	github.com/aws/aws-sdk-go-v2/service/s3 v1.30.1
	github.com/aws/aws-sdk-go-v2/service/ssm v1.37.2
	github.com/aws/aws-sdk-go-v2/service/xray v1.16.14
	github.com/aws/aws-xray-sdk-go v1.8.1
	github.com/cenkalti/backoff/v4 v4.2.1
	github.com/google/uuid v1.3.0
	github.com/mitchellh/mapstructure v1.5.0
	github.com/prozz/aws-embedded-metrics-golang v1.2.0
	github.com/qri-io/jsonschema v0.2.1
	github.com/shirou/gopsutil/v3 v3.23.3
	github.com/stretchr/testify v1.8.3
	go.opentelemetry.io/contrib/propagators/aws v1.17.0
	go.opentelemetry.io/otel v1.16.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.16.0
	go.opentelemetry.io/otel/sdk v1.16.0
	go.opentelemetry.io/otel/trace v1.16.0
	go.uber.org/multierr v1.9.0
	golang.org/x/exp v0.0.0-20230224173230-c95f2b4c22f2
	golang.org/x/sys v0.8.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/andybalholm/brotli v1.0.4 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.4.10 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.13.10 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.38 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.32 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.28 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.0.18 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodbstreams v1.13.20 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.9.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.1.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.7.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.13.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.12.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.14.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.18.2 // indirect
	github.com/aws/smithy-go v1.14.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.7.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/compress v1.15.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/qri-io/jsonpointer v0.1.1 // indirect
	github.com/shoenig/go-m1cpu v0.1.4 // indirect
	github.com/tklauser/go-sysconf v0.3.11 // indirect
	github.com/tklauser/numcpus v0.6.0 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.34.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.16.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.16.0 // indirect
	go.opentelemetry.io/otel/metric v1.16.0 // indirect
	go.opentelemetry.io/proto/otlp v0.19.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	golang.org/x/mod v0.8.0 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/tools v0.6.0 // indirect
	google.golang.org/genproto v0.0.0-20230306155012-7f2fa6fef1f4 // indirect
	google.golang.org/grpc v1.55.0 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
)
