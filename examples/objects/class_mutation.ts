class MutableCounter {
  value: number = 1;

  bump(): number {
    this.value = this.value + 1;
    return this.value;
  }
}

const counter = new MutableCounter();
console.log(counter.bump());
console.log(counter.bump());
