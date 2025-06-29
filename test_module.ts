// Simple module test
export const message = "Hello from module!";
export function greet(name: string): string {
    return `Hello, ${name}!`;
}

export default function defaultGreet(): string {
    return "Default greeting!";
}