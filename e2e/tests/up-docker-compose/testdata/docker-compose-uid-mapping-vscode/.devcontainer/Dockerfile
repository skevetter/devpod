FROM ubuntu:latest

RUN useradd -m -s /bin/bash vscode && \
    echo 'vscode ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers

RUN mkdir -p /workspaces/project && \
    chown -R vscode:vscode /workspaces
