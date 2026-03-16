// expect: ok
type Level = "low" | "medium" | "high";

interface Config {
  level?: Level;
}

function apply(cfg: Config): string {
  if (cfg.level !== undefined) {
    return cfg.level;
  }
  return "default";
}

let result = apply({ level: "high" });
let result2 = apply({});
if (result === "high" && result2 === "default") {
  "ok";
} else {
  "fail";
}
