const crypto = require("crypto");

class AuthManager {
  constructor() {
    this.token = crypto.randomUUID();
  }

  getToken() {
    return this.token;
  }

  verifyRequest(req) {
    const authHeader = req.headers.authorization;
    if (!authHeader) return false;
    const token = authHeader.split(" ")[1];
    return token === this.token;
  }
}

module.exports = new AuthManager();
