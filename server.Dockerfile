FROM node:20-alpine3.18
WORKDIR /app
COPY package.json .npmrc /app
RUN env NODE_ENV=production corepack pnpm install --prod --no-optional \
 && rm -rf ~/.cache/pnpm ~/.local/share/pnpm/store
COPY server/ /app/server/
CMD ["corepack", "pnpm", "-s", "serve"]
