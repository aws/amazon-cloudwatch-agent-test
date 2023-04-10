module github.com/aws/amazon-cloudwatch-agent-test

go 1.18

// Avoid checksum mismatch for go-collectd https://github.com/collectd/go-collectd/issues/94
replace collectd.org v0.5.0 => github.com/collectd/go-collectd v0.5.0

require (
	collectd.org v0.5.0
	github.com/DataDog/datadog-go v4.8.3+incompatible
	github.com/aws/aws-sdk-go v1.44.188
	github.com/aws/aws-sdk-go-v2 v1.17.8
	github.com/aws/aws-sdk-go-v2/config v1.18.21
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.10.0
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.13.2
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.11.62
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.25.2
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.15.20
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.18.2
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.77.0
	github.com/aws/aws-sdk-go-v2/service/ecs v1.23.2
	github.com/aws/aws-sdk-go-v2/service/s3 v1.31.3
	github.com/aws/aws-sdk-go-v2/service/ssm v1.33.0
	github.com/cenkalti/backoff/v4 v4.2.0
	github.com/google/uuid v1.3.0
	github.com/mitchellh/mapstructure v1.5.0
	github.com/qri-io/jsonschema v0.2.1
	github.com/stretchr/testify v1.8.1
	go.uber.org/multierr v1.9.0
	golang.org/x/exp v0.0.0-20230224173230-c95f2b4c22f2
	golang.org/x/sys v0.2.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.4.10 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.13.20 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.1.32 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.4.26 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.3.33 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.0.24 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodbstreams v1.13.20 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.9.11 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.1.27 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.7.22 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.9.26 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.14.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.12.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.14.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.18.9 // indirect
	github.com/aws/smithy-go v1.13.5 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/qri-io/jsonpointer v0.1.1 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	golang.org/x/mod v0.6.0 // indirect
	golang.org/x/tools v0.2.0 // indirect
)
