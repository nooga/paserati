class Map<K, V> {
    private entries: Array<[K, V]> = [];
    
    set(key: K, value: V): void {
        // Find existing entry
        for (let i = 0; i < this.entries.length; i++) {
            if (this.entries[i][0] === key) {
                this.entries[i][1] = value;
                return;
            }
        }
        // Add new entry
        this.entries.push([key, value]);
    }
    
    get(key: K): V | undefined {
        for (let entry of this.entries) {
            if (entry[0] === key) {
                return entry[1];
            }
        }
        return undefined;
    }
}

let map: Map<string, number> = new Map<string, number>();
map.set("test", 42);
let value = map.get("test");
console.log(value);

// expect: 42