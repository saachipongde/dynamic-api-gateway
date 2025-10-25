const express = require('express');
const app = express();
const PORT = 3000; // Port inside the container

app.get('/', (req, res) => {
  console.log('Request received for service-b');
  res.json({
    service: "service-b",
    status: "ok"
  });
});

app.listen(PORT, () => {
  console.log(`Service-B listening on port ${PORT}`);
});