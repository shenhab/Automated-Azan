#!/usr/bin/env bash
# Install uv if not present
pip install --user uv
export PATH="$HOME/.local/bin:$PATH"

# Create virtual environment and install dependencies
uv venv environment
source environment/bin/activate
uv pip install -e .