import { checkSnapshots } from "../check/run.js";

const r = checkSnapshots();
(r.ok ? console.log : console.error)(r.message);
process.exit(r.ok ? 0 : 1);
