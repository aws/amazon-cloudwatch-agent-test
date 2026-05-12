FROM public.ecr.aws/aws-ec2/amazon-ec2-metadata-mock:v1.13.0 AS aemm
FROM busybox:musl
COPY --from=aemm /ec2-metadata-mock /ec2-metadata-mock
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
