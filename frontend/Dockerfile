# Multi-stage: Node builds the static React bundle, then nginx serves it. The final
# image has no Node — just nginx + the built HTML/JS/CSS.

# ---- build stage ----
FROM node:22-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci                 # clean, reproducible install from package-lock.json
COPY . .
RUN npm run build          # outputs static files to /app/dist

# ---- serve stage ----
FROM nginx:alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
