class InitializedPoint {
  x: number = 20;
  y: number = 22;

  sum(): number {
    return this.x + this.y;
  }
}

console.log(new InitializedPoint().sum());
