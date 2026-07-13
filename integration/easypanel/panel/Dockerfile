# Fake EasyPanel panel image: Node + real lmdb module + seeded LMDB store at the
# same path the real panel uses, so easypanel-migrate's docker-exec LMDB token
# extraction and tRPC/REST migrate paths are exercised against a faithful shape.
FROM node:20-alpine
WORKDIR /app
# Install the real lmdb module the extraction script requires at /app/node_modules/lmdb.
RUN npm init -y >/dev/null 2>&1 && npm install lmdb@3.4.4 --no-audit --no-fund
RUN mkdir -p /etc/easypanel/data
COPY server.js /app/server.js
EXPOSE 3000
CMD ["node", "/app/server.js"]
