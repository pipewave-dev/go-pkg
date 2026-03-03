#!/bin/bash

PATH="$PATH:$(go env GOPATH)/bin"

binary_name="buf"
min_version="1.50"

if ! command -v "$binary_name" &> /dev/null; then
  echo "Binary '$binary_name' is not installed. Please install at... https://buf.build/docs/installation"

else
  # If the binary is installed, check its version using '--version'
  installed_version=$("$binary_name" --version)
  echo "'$binary_name' version is $installed_version"

  if ! printf '%s\n%s\n' "$min_version" "$installed_version" | sort -V -C; then
    echo "$binary_name required version >= $min_version"
    echo "$binary_name version is $installed_version . Please update newest version. https://buf.build/docs/installation"
  else
    $binary_name generate && echo "Proto files generated successfully"
  fi

fi
