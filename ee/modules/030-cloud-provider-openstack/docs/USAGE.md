---
title: "Cloud provider — OpenStack: usage"
---

Below are two simple examples of OpenStack cloud provider configuration.

## Example 1
```yaml
apiVersion: deckhouse.io/v1
kind: OpenStackInstanceClass
metadata:
  name: test
spec:
  flavorName: m1.large
```

## Example 2

```yaml
cloudProviderOpenstack: |
  connection:
    authURL: https://test.tests.com:5000/v3/
    domainName: default
    tenantName: default
    username: jamie
    password: nein
    region: HetznerFinland
  externalNetworkNames:
  - public
  internalNetworkNames:
  - kube
  instances:
    sshKeyPairName: my-ssh-keypair
    securityGroups:
    - default
    - allow-ssh-and-icmp
  zones:
  - zone-a
  - zone-b
  tags:
    project: cms
    owner: default
```
