const express = require('express');
const app = express();
const PORT = 3000; // Port inside the container

app.get('/', (req, res) => {
  console.log('Request received for service-a');
  res.json({
    service: "service-a",
    status: "ok"
  });
});

app.listen(PORT, () => {
  console.log(`Service-A listening on port ${PORT}`);
});