const maxmind = require("maxmind");
const path = require("path");
const dbPath = "C:\\Users\\andbe\\AppData\\Roaming\\resultproxy\\GeoLite2-Country.mmdb"; // Assuming userData is here based on typical Electron
maxmind.open(dbPath).then(lookup => {
  const loc = lookup.get("45.85.162.156");
  console.log(loc ? loc.country.iso_code : "none");
});
