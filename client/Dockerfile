FROM oven/bun:1.4.2-debian AS build
WORKDIR /src
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile
COPY tsconfig.json biome.json README.md ./
COPY client ./client
RUN bun run build:client

FROM nginx:1.29.3-alpine
COPY client/nginx.conf /etc/nginx/nginx.conf
COPY client/nginx.vhosts.conf.template /etc/nginx/templates/default.conf.template
COPY client/snippets/ /etc/nginx/snippets/
COPY deploy/nginx-entrypoint.sh /usr/local/bin/hackathon-nginx-entrypoint
COPY --from=build /src/dist /usr/share/nginx/html
RUN chmod 0555 /usr/local/bin/hackathon-nginx-entrypoint \
    && rm -f /etc/nginx/conf.d/default.conf
ENV PUBLIC_BASE_URL="" PLANNER_BASE_URL=""
EXPOSE 80 443
ENTRYPOINT ["/usr/local/bin/hackathon-nginx-entrypoint"]
