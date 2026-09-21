// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../../common"
}

module "iam" {
  source                  = "../iam"
  name_prefix             = "cwa-azurevmwin-integ"
  testing_id              = module.common.testing_id
  principal_id            = azurerm_windows_virtual_machine.cwagent.identity[0].principal_id
  azure_oidc_provider_arn = var.azure_oidc_provider_arn
  azure_token_audience    = var.azure_token_audience
}
