FROM --platform=linux/amd64 node:20-bookworm AS build

WORKDIR /src
COPY package.json ./
COPY package-lock.json ./
COPY packages/ui/package.json packages/ui/
COPY demo-app/package.json demo-app/
COPY admin-dashboard/package.json admin-dashboard/
RUN npm ci
COPY packages/ui/ packages/ui/
COPY demo-app/ demo-app/
RUN mkdir -p demo-app/public && : > demo-app/public/.keep
RUN npm --workspace demo-app run build

FROM --platform=linux/amd64 node:20-bookworm-slim

ENV NODE_ENV=production
ENV PORT=3000

WORKDIR /src
COPY package.json ./
COPY package-lock.json ./
COPY packages/ui/package.json packages/ui/
COPY demo-app/package.json demo-app/
COPY admin-dashboard/package.json admin-dashboard/
RUN npm ci --omit=dev

WORKDIR /src/demo-app
COPY --from=build /src/demo-app/.next ./.next
COPY --from=build /src/demo-app/public ./public

EXPOSE 3000

CMD ["npm", "run", "--workspace", "demo-app", "start", "--", "-p", "3000"]
