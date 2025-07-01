// Math module with exported types and functions
export interface Vector2D {
    x: number;
    y: number;
}

export function add(a: Vector2D, b: Vector2D): Vector2D {
    return { x: a.x + b.x, y: a.y + b.y };
}

export function magnitude(v: Vector2D): number {
    return Math.sqrt(v.x * v.x + v.y * v.y);
}

export const ZERO: Vector2D = { x: 0, y: 0 };
export const UNIT_X: Vector2D = { x: 1, y: 0 };
export const UNIT_Y: Vector2D = { x: 0, y: 1 };

// export default Vector2D;