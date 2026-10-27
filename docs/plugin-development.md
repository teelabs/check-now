# Plugin development

Create an offline-safe plugin with `check-now plugin create <id>`. Required fields are ID, display name, and one-sentence description. Optional command, source URL, and manager fields can be left blank. Runtime defaults to `shell`; capabilities default to `current,refresh`.

Non-interactive example:

```sh
check-now plugin create nodejs --name 'Node.js' --description 'Checks Node.js installations' --tool node --source-url https://nodejs.org --runtime shell --manager fnm --capability current,refresh --non-interactive
```

The command creates `plugin.sh`, `README.md`, `AGENTS.md`, and executable `test.sh` under `plugins/<id>`. Existing folders require `--force`.

The generated `plugin.sh` is intentionally conservative and self-contained. Implement detection, installed-version lookup, release comparison, manager identification, and safe update arguments in that file. Do not add a helper executable that the host must know about. The host discovers the folder and speaks only the protocol.

For an interactive shell, the normal development loop is:

```sh
check-now plugin create example --name Example --description 'Checks Example' \
  --tool example --source-url https://example.invalid \
  --non-interactive
cd plugins/example
./test.sh
cd ../..
make test
check-now list
check-now doctor
```

The generated README is the plugin's implementation checklist. Record the real source endpoint, installation managers, version policy, and every safe update argument. If any of those are uncertain, leave the result `unknown` and explain why.

For AI-assisted work, read the generated instructions, the plugin protocol, and existing plugins first. Write a deterministic local fixture test before detection or update logic. Never use live network APIs in tests or execute an update command while implementing or testing.

