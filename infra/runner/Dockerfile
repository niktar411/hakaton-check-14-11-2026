FROM gitlab/gitlab-runner:alpine-v18.5.0
RUN apk add --no-cache docker-cli openssh-client
COPY deploy/gitlab-runner-entrypoint.sh /usr/local/bin/hackathon-runner-entrypoint
RUN chmod 0555 /usr/local/bin/hackathon-runner-entrypoint
ENTRYPOINT ["/usr/local/bin/hackathon-runner-entrypoint"]
