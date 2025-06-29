class Map<K, V> {
    get(key: K): V | undefined {
        return undefined;
    }
}

let map: Map<string, number> = new Map<string, number>();
map.get("test");