#!/bin/bash

# Replace the bind_address in config.yaml dynamically to use Render's $PORT
sed -i "s/bind_address: .*/bind_address: \"0.0.0.0:$PORT\"/" config.yaml

# Now start GoToSocial with updated config
./gotosocial --config-path config.yaml
