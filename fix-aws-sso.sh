#!/bin/bash
# Fix AWS SSO configuration
# The issue is usually with SSO registration scopes or region mismatch

echo "Fixing AWS SSO configuration..."
echo ""
echo "When running 'aws configure sso', use these values:"
echo "  SSO session name: sso"
echo "  SSO start URL: https://d-9067d97ba9.awsapps.com/start/#/"
echo "  SSO region: ap-south-1 (or try us-east-1 if this doesn't work)"
echo "  SSO registration scopes: [Press Enter for default, or type: sso:account:access]"
echo ""
echo "If that doesn't work, try manually configuring:"
echo ""
echo "Run: aws configure sso --profile default"
echo ""
echo "Or configure manually by editing ~/.aws/config:"
echo ""
cat << 'CONFIG'
[profile default]
sso_start_url = https://d-9067d97ba9.awsapps.com/start/#/
sso_region = ap-south-1
sso_account_id = YOUR_ACCOUNT_ID
sso_role_name = YOUR_ROLE_NAME
region = ap-south-1
CONFIG
