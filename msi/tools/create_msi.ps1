# get the version
$version=$args[0]
$bucket=$args[1]

# create msi
candle.exe -ext WixUtilExtension.dll ./amazon-cloudwatch-agent.wxs
light.exe -ext WixUtilExtension.dll ./amazon-cloudwatch-agent.wixobj

# upload to s3
$bucketPath = "s3://$bucket/integration-test/packaging/$version/amazon-cloudwatch-agent.msi"
if ($version -eq "") {
    # This is a prod, nonprod, or nightly build.
    $bucketPath = "s3://$bucket/$version/amazon-cloudwatch-agent.msi"
}
aws s3 cp ./amazon-cloudwatch-agent.msi $bucketPath
Write-Host "s3 for msi is $bucketPath"