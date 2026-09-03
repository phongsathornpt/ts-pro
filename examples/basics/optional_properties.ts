interface Config {
  host: string;
  port?: number;
  secure?: boolean;
}

interface Circle {
  kind: string;
  radius: number;
}

interface Square {
  kind: string;
  size: number;
}

type Shape = Circle | Square;

export function testOptionalAndUnion(): void {
  const c1: Config = { host: "localhost", port: 8080, secure: true };
  const c2: Config = { host: "remote" };
  console.log(c1.host);
  console.log(c1.port);
  console.log(c1.secure);
  console.log(c2.host);
  console.log(c2.port);
  console.log(c2.secure);

  const shape1: Shape = { kind: "circle", radius: 10 };
  const shape2: Shape = { kind: "square", size: 20 };
  console.log(shape1.kind);
  console.log(shape2.kind);
}
testOptionalAndUnion();
