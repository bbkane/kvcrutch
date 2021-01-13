While the README is a lovely facade meant to entice users, these are
development notes with the construction beams exposed.

# Test for now. Someday I'll have real tests :)

go run . certificate create \
    --id 'test-create-flags' \
    --subject 'CN=bbkane.com' \
    --san bbkane.com \
    --san www.bbkane.com \
    --tag 'bkey=bvalue' \
    --validity 12 \
    --enabled

# TODO: cmd plans

This is me designing what the CLI will look like. Subject to change

```
kvcrutch
    --vault-name
    create
        --disabled
        --tags [key=value, ...]
        --name
        --validity
        --cn
        --san ... ...
    new-version
        --add-san ...
        --rm-san ...
        --add-tag ...
        --rm-tag ...
        --validity
    list  # actually list everything
    config edit
```

# TODO

- add better help to commands and examples to flags
- document certifcate new-version
- document config better
- blog post: What I don't like about Azure Key Vault (with workarounds)
  - GUI
    - manually creating a new version drops tags
    - can't add tags on creation
    - can't link to all versions of a cert
    - can't search keyvault by SAN
    - I don't think you can attach emails to certificates
  - CLI
    - See README for kvcrutch
  - Rest API
  - Go API
  - Other
    - enabling soft delete means you can't delete secrets when you delete certs
- validate subject CN in SANs
