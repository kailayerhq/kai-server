FROM node:20-alpine AS build

RUN apk add --no-cache git

WORKDIR /src

# Clone OSS repo for docs markdown and CHANGELOG
RUN git clone --depth 1 https://github.com/kaicontext/kai.git /oss

# Copy VitePress site config
COPY docs-site/ ./docs-site/

# Copy OSS docs into the VitePress source directory
RUN cp /oss/docs/*.md ./docs-site/
RUN cp /oss/CHANGELOG.md ./docs-site/changelog.md 2>/dev/null || echo "# Changelog\n\nNo changelog available yet." > ./docs-site/changelog.md

WORKDIR /src/docs-site
RUN npm ci
RUN npm run build

FROM nginx:alpine
COPY --from=build /src/docs-site/.vitepress/dist /usr/share/nginx/html
EXPOSE 80
