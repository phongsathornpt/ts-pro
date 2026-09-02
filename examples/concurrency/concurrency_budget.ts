const budgetChannel = channel<number>(2);

const budgetHeavy = spawn((): void => {
  let i = 0;
  while (i < 5000) {
    i = i + 1;
  }
  channelSend(budgetChannel, 1);
});

const budgetQuick = spawn((): void => {
  channelSend(budgetChannel, 2);
});

console.log(channelRecv(budgetChannel));
console.log(channelRecv(budgetChannel));
join(budgetHeavy);
join(budgetQuick);
