FROM node:12-slim

WORKDIR /usr/src/app
COPY package*.json ./
RUN npm install --only=production
COPY gs1200-exporter.js ./

ENV GS1200_ADDRESS 192.168.1.3
ENV GS1200_PASSWORD 1234
ENV GS1200_PORT 9707

EXPOSE $GS1200_PORT
CMD [ "node", "gs1200-exporter.js" ]
