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

# Test for now. Someday I'll have real tests :)

go run . certificate create \
    --id 'test-create-flags' \
    --subject 'CN=bbkane.com' \
    --san bbkane.com \
    --san www.bbkane.com \
    --tag 'bkey=bvalue' \
    --validity 12 \
    --enabled

# TODO

- embed config into a static package... VSCode isn't respecting my spaces instead of tabs within the string
- add goreleaser
- make other commands (cert new-version, cert list)
- document config better

