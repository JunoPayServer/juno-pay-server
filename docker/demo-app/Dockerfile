FROM --platform=linux/amd64 node:20-bookworm AS build

WORKDIR /src/demo-app
COPY demo-app/package.json demo-app/package-lock.json ./
RUN npm ci
COPY demo-app/ ./
RUN mkdir -p public
RUN npm run build

FROM --platform=linux/amd64 node:20-bookworm-slim

ENV NODE_ENV=production
ENV PORT=3000

WORKDIR /app

COPY demo-app/package.json demo-app/package-lock.json ./
RUN npm ci --omit=dev

COPY --from=build /src/demo-app/.next ./.next
COPY --from=build /src/demo-app/public ./public

EXPOSE 3000

CMD ["npm", "run", "start", "--", "-p", "3000"]
