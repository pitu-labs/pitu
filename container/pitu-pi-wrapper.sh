#!/bin/bash
set -e

# 1. Prepare MCP config from environment if present
if [ -n "$PI_CONFIG_CONTENT" ]; then
  mkdir -p ~/.pi/agent
  echo "$PI_CONFIG_CONTENT" > ~/.pi/agent/mcp.json
fi

# 2. Extract prompt from Pitu JSON input
INPUT_FILE=$1
if [ -f "$INPUT_FILE" ]; then
  # Extract text using jq
  PROMPT=$(jq -r .text "$INPUT_FILE")
  
  # 3. Run pi in print mode (-p) with the prompt
  # We use --session to force the log to a known location for Pitu's harness to watch.
  pi -p --session "/workspace/memory/log.jsonl" "$PROMPT"
else
  # Fallback: if no input file, just run sleep infinity (though Pitu handles this via handle ID)
  sleep infinity
fi
