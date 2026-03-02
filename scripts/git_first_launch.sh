#!/bin/bash

# Check if GIT_EMAIL is not empty
if [ -n "$GIT_EMAIL" ]; then
  git config --global user.email "$GIT_EMAIL"
  echo "Git global user.email set to $GIT_EMAIL"
else
  echo "GIT_EMAIL is empty or not set"
fi

# Check if GIT_USER is not empty
if [ -n "$GIT_USER" ]; then
  git config --global user.name "$GIT_USER"
  echo "Git global user.name set to $GIT_USER"
else
  echo "GIT_USER is empty or not set"
fi



