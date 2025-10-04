# Discord Fansly Notifier


> [!IMPORTANT]
> The project has been moved to [this repo](https://github.com/NotiFansly/notifansly-bot)


> [!WARNING]
> This is an unofficial fansly bot for discord to be notified of new post and when a creator goes live on the platform. This only works if the creators profile either has no requirements for being viewed or just needing to be followed to view. No actual content is leaked via this bot if used via provided bot link or if self ran with a basic account just following.

> [!NOTE]
> Self hosted versions of the bot **may** display post in servers if the account used is subscribed but no actual photos or videos are displayed in the embed messages. I did plan on having any preview images/videos be sent but didn't seem to work, and I refused to have them sent as attachments.

[Add to your server](https://notifansly.xyz/)

## TODO:

- [ ] Hack together way to provide roles based on sub/follow per server
- [x] Fix following logic to still add if account has no pfp and just let it be empty in the embed
- [x] Possibly separate live and post notification
    - [x] Allow enabling and disabling one or the other

## Running The Bot Yourself 

Firstly download or clone the repository:

```bash
git clone github.com/fvckgrimm/discord-fansly-notify && cd discord-fansly-notify

# Create the .env to configure
cp .env-example .env

# Running the program
go run .

# Building Binary
go build -v -ldflags "-w -s" -o fansly-notify ./cmd/fansly-notify/

# Running the binary 
./fansly-notify
```

## Configuring The .env File

To run this bot you will need to get BOTH your discord bots token and other items, and your fansly account token.

### Discord Bot token 

To get the needed discord values for the .env file, you can read and follow the instructions from [discords developrs doc's](https://discord.com/developers/docs/quick-start/getting-started#step-1-creating-an-app) 

### Fansly Token

Recommended method:
1. Go to [fansly](https://fansly.com) and login and open devtools (ctrl+shift+i / F12)
2. In devtools, go to the Console Tab and Paste the following: 
```javascript
console.clear(); // cleanup console
const activeSession = localStorage.getItem("session_active_session"); // get required key
const { token } = JSON.parse(activeSession); // parse the json data
console.log('%c➡️ Authorization_Token =', 'font-size: 12px; color: limegreen; font-weight: bold;', token); // show token
console.log('%c➡️ User_Agent =', 'font-size: 12px; color: yellow; font-weight: bold;', navigator.userAgent); // show user-agent
```

## Disclaimer 
> [!CAUTION]
> Use at your own risk. The creator of this program is not responsible for any outcomes that may take place upon the end users' account for using this program. This program is not affiliated or endorsed by "Fansly" or Select Media LLC, the operator of "Fansly". 
