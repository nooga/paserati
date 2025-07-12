// Module that exports various types of enums

export enum Direction {
    Up,
    Down,
    Left,
    Right
}

export const enum Color {
    Red,
    Green,
    Blue
}

export enum Status {
    Loading = "loading",
    Success = "success",
    Error = "error"
}

export enum Mixed {
    A,
    B = "hello",
    C = 5,
    D
}

// Note: Export functions using enum types in signatures are not yet supported
// This is a known limitation that will be addressed in future updates