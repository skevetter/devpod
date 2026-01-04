FROM ubuntu

RUN DEBIAN_FRONTEND=noninteractive apt-get update && apt-get install -y \
    curl apache2 git openssh-server \
    && rm -rf /var/lib/apt/lists/*

RUN chown -R www-data /var/www

RUN chsh -s /bin/bash www-data

CMD ["tail", "-f", "/dev/null"]
