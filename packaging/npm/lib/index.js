// portcheck-cli as a library: ask the bundled portcheck who holds a port.
//
//   const { binary, check, describe } = require("portcheck-cli");
//   const data = await check(3000, await binary());
//   console.log(describe(3000, data).lines.join("\n"));
"use strict";

const { binary, check, describe, kill } = require("./check.js");
const { ensureBinary } = require("./install.js");

module.exports = { binary, check, describe, ensureBinary, kill };
