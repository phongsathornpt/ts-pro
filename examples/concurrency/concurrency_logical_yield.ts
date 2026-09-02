const yieldSignal = channel<boolean>(0);

const yieldWaiter = spawn((): void => {
  channelRecv(yieldSignal);
  console.log(2);
});

const yieldProducer = spawn((): void => {
  channelSend(yieldSignal, true);
  console.log(1);
  yieldNow();
  console.log(3);
});

join(yieldWaiter);
join(yieldProducer);
