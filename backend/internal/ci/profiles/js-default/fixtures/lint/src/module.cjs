const path = require("node:path");

module.exports = function resolve(name) {
  return path.join(__dirname, name);
};
