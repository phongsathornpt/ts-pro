interface ProbeDynamicThisBox {
  value: number;
  add: (this: ProbeDynamicThisBox, delta: number) => number;
  plain: (delta: number) => number;
}

const probeDynamicThisBox: ProbeDynamicThisBox = {
  value: 40,
  add: function (this: ProbeDynamicThisBox, delta: number): number {
    return this.value + delta;
  },
  plain: (delta: number): number => 40 + delta,
};

const probeDynamicThisValue: any = probeDynamicThisBox;
console.log(probeDynamicThisValue.add(2));
console.log(probeDynamicThisValue.plain(2));
