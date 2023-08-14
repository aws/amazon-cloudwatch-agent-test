# Header above mocked_servers cert in bundle
CERT_HEADER=mockserver

CONFIG_PATH=openssl.conf
CERT_PATH=certificates/ssl/certificate.crt
KEY_PATH=certificates/private.key
BUNDLE_PATH=certificates/ssl/ca-bundle.crt
.PHONY: clean
clean: 
	rm -rf $(CERT_PATH) $(BUNDLE_PATH) $(KEY_PATH)
	touch $(CERT_PATH) $(BUNDLE_PATH) $(KEY_PATH)
.PHONY: update-certs
update-certs: clean gen-cert update-bundle

# Expects mocked_servers cert to be the last cert in the bundle
# Cuts until the first instance of "mocked_servers" in the bundle
# and concatenates it with the current cert
.PHONY: update-bundle
update-bundle:
	sed /$(CERT_HEADER)/q $(BUNDLE_PATH) | cat - $(CERT_PATH) > $(BUNDLE_PATH).tmp && \
	mv $(BUNDLE_PATH).tmp $(BUNDLE_PATH)

# Generates the annual cert and private key using the config
.PHONY: gen-cert
gen-cert:
	openssl req -config $(CONFIG_PATH) -new -x509 -nodes -days 365 -out $(CERT_PATH) -keyout $(KEY_PATH)