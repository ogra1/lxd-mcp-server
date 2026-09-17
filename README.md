  An MCP server that bridges between an LLM and LXD containers on the same host.

  The package will autmatically start a daemon listening on port 8005 by default.
  Point your LLM to http://127.0.0.1:8005/mcp in its MCP settings and make sure 
  to connect the lxd interface for this snap with:

    sudo snap connect lxd-mcp-server:lxd lxd:lxd

  So that the bridge can actually talk to your lxd instance.

  The MCP server provides 9 tools to your LLM that you can use:
  
    - list_containers (lists existing containers)
    - create_container (creates a new container, with <name> and <release>)
    - start_container (fires up the named container)
    - stop_container (stops the named container)
    - execute_command (runs a command inside the container)
    - push_file (copies files into the container)
    - pull_file (copies files out of the container)
    - limit_resources (allows to limit ram and cpu usage for a container)

  There are also two config options you can use with snap set:
  
    - port (defaults to 8005)
    - debug (defaults ot being off)

  Note that you might need to enable the built in LLM proxy in some LLMs
  (i.e. in llama-server this is the "Use llama-server proxy" option in the 
  llama-ui)  

