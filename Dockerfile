FROM node:12-alpine AS BUILD
WORKDIR /usr/src/app
COPY package*.json ./
COPY gs1200-exporter.js ./
RUN npm install --only=production
RUN npm prune --production

FROM node:12-alpine
COPY --from=BUILD /usr/src/app /

ENV GS1200_ADDRESS 192.168.1.3
ENV GS1200_PASSWORD 1234
ENV GS1200_PORT 9707

EXPOSE $GS1200_PORT
CMD [ "node", "gs1200-exporter.js" ]
