FROM node:22-alpine AS build

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm run build
RUN npm prune --omit=dev

FROM node:22-alpine AS runtime

ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    API_HOST=127.0.0.1 \
    API_PORT=8081 \
    NEXT_HOST=0.0.0.0 \
    NEXT_PORT=3000 \
    UDI_API_ORIGIN=http://127.0.0.1:8081 \
    DATA_FILE=/app/data/udi.json

WORKDIR /app
COPY --from=build --chown=node:node /app/node_modules ./node_modules
COPY --from=build --chown=node:node /app/package.json ./package.json
COPY --from=build --chown=node:node /app/server.mjs ./server.mjs
COPY --from=build --chown=node:node /app/lib ./lib
COPY --from=build --chown=node:node /app/scripts ./scripts
COPY --from=build --chown=node:node /app/public ./public
COPY --from=build --chown=node:node /app/.next ./.next
RUN mkdir -p /app/data && chown node:node /app/data

USER node
EXPOSE 3000
CMD ["npm", "run", "start"]
