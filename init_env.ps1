# Install uv if not present
pip install uv

# Create virtual environment and install dependencies
uv venv environment
.\environment\Scripts\activate.ps1
uv pip install -e .