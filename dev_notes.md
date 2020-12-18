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

- add goreleaser
- make other commands (cert new-version, cert list)
- document config better

