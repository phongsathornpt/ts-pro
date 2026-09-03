export enum Status {
  Ok = 200,
  NotFound = 404
}

export enum Direction {
  Up,
  Down,
  Left,
  Right
}

export function move(dir: Direction, steps: number = 1): string {
  if (dir === Direction.Up) {
    return `moving Up by ${steps}`;
  }
  if (dir === Direction.Down) {
    return `moving Down by ${steps}`;
  }
  if (dir === Direction.Left) {
    return `moving Left by ${steps}`;
  }
  return `moving Right by ${steps}`;
}

export function greet(name: string, greeting: string = "Hello", title?: string): string {
  if (title !== undefined) {
    return `${greeting}, ${title} ${name}!`;
  }
  return `${greeting}, ${name}!`;
}

// 1. Enums
console.log(Status.Ok);
console.log(Status.NotFound);
console.log(Direction.Up);
console.log(Direction.Down);
console.log(Direction.Left);
console.log(Direction.Right);

// 2. Default parameters
console.log(move(Direction.Up));
console.log(move(Direction.Down, 5));

// 3. Default and optional parameters
console.log(greet("Alice"));
console.log(greet("Bob", "Hi"));
console.log(greet("Watson", "Greetings", "Dr."));
