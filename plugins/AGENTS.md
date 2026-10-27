# Plugin repository rules

- A plugin folder name is its canonical ID; `plugin.sh` is the only executable entrypoint.
- Metadata `id` must match the folder name and optional fields must be omitted when unavailable.
- Refresh responses use the fixed protocol schema and supported statuses.
- Unknown installation methods must not produce guessed update commands.
- Plugin tests are deterministic and offline. Inspect existing plugin conventions before adding detection logic.
- Run the plugin smoke test and relevant host tests before reporting completion.

