# TODO

## Features

- [ ] Add an implode command to remove the local repository.
- [ ] Add configuration (local.toml)to change some files to the fork branch.
- [ ] After init clones the main branch, check if there is a .config/scadufax/config.toml, if not, warn the user that it is missing.
- [ ] Add a check to see if the config.toml has a fork entry, and alert the user about it. It should be in the local.toml.
- [ ] Add a check to see if the config.toml has a home_dir entry, and alert the user about it. It should be in the local.toml.
- [ ] Add a check to see if the config.toml has a local_dir entry, and alert the user about it. It should be in the local.toml.
- [ ] Add a check to see if the config.toml has a confirm entry, and alert the user about it. It should be in the local.toml.

## Bugs

- [ ] After init, the fork branch is not pushed to the remote repository.
- [ ] Cloning links change their content, and make the .local repo dirty.
