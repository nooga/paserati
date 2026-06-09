// expect: 1
let config: { [key: string]: boolean | { prop: string } } = {
    works: { prop: "ok" },
};

if (typeof config["works"] !== "boolean") {
    config.works.prop = "test";
    config["works"].prop = "test";
}

if (typeof config.works !== "boolean") {
    config["works"].prop = "test";
    config.works.prop = "test";
}

1;
