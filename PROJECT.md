# vocabulary trainer

- implement a vocabulary trainer as described below
- backend written in go
- frontend using JS/HTML/CSS
- can run locally in docker
- store data in sqlite
- user is allowed to add vocabulary in chinese, pingin and English
- user can learn vocabulary: randomly show a vocabulary in English or Chinese or Chinese + pingin, if English is shown, user needs to type in the Chinese translatin, if Chinese (with or without pingin) then the user needs to type in English
- for each vocabulary track how often the user gave the correct or incorrect answer
- when selecting the next vocabulary for training, show vocabulary with a higher error rate with a higher probability