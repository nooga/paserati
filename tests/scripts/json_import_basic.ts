// expect: hello-42
import data from "./data.json" with { type: "json" };
data.message + "-" + data.value;
