# Marketing / Readme

- drop-in replacement for sail, can import sail config
- full development experience with selectable services 
- queue workers (ad hoc/preconfigured) with auto reload on code change
- choice of PHP versions, runtimes, services... 
- PHP-less Laravel installer
- Docker support
- Contextual aliases
- Minimal dependencies: go and docker
- etc...

## About

Frank is an elevated Laravel application development tool that comes with batteries included to help you create your app without the mental overhead.

### What overhead?

My first objective with **Frank** was to bridge documentation commands to actual shell commands, as I mainly use Sail to develop Laravel apps. Every commands of `php artisan`, `composer require`,  etc. had to be either prefixed or modified with `sail`. 

From this basis, _Frank_ was born, but there's actually more to that.

#### The Pythonic way

Having had experience with Python development, `python3 -m venv venv` brought custom aliases scoped to the project, by running `venv/bin/activate`:
- `python` would alias to the contained python version
- `pip` as well
- ... and any binary/library installed in that python project

I wanted something similar with PHP using Sail, so the earliest version of this project was just using a `justfile` and a few shell tricks to get aliased commands talking to Docker.

#### PHP versions per process (CLI/FPM, ...)

Having PHP installed locally was also a source of confusion: the PHP CLI is in _this_ version, while the PHP-FPM is in _that_ version, improper `composer` setup would require `sudo` (I fixed that but just to cite things), and when using `sail` it was too easy to forget using the sail prefixes and run the local php (in a different version than the Sail PHP of course) crashing with errors.

#### Collaboration in a professional context

At work, more and more projects were using queues and when developing locally the experience is pretty terrible: many shell panes with one running `npm run dev`, another with `php artisan queue:worker --queue=1,2,3` (yes `queue:listen` exists but inconsistent experience), and maybe another running the scheduler. Oh, and you had to re-run the queues every now and then after editing the code. 
Also some colleagues at my current workplace who are not accustomed to using queues, were forgetting to run them losing time/energy on why things don't work.

On another professional setting, people usually work with their shortcuts and that is fine. For instance, I use `pa` (`php artisan`) which auto-detects either sail or local php (redundant now), but some other colleagues might use `arti` or whatever. You can always dictate the full command when collaborating with a colleague, but what about "oh just run this" and it _just works_? Commitable aliases per project, almost making them wish they were here by default!

#### The Laravel Way

I also know that the Laravel community is somewhat opinionated (a bit like the "Pythonic" term in a way), so instead of changing people's habits too much, I want to offer a drop-in/drop-out experience, covering various use cases but without ever locking the developer inside the tool that is Frank. You can use it solely to install a Laravel app via Docker and use Laravel Herd if that's your jam, or to create a Sail app directly. At its core, Frank is just a "Docker Laravel installer".

### And now... Frank!

Should you choose to use Frank, a brief overview of what's included: you can customize the runtime between Frankenphp or the classic PHP-FPM, select your PHP version, your preferred node package manager, and how many queue workers you want, among other things. Everything changeable at any time, since everything is just containers. 

Another benefit from using `frank` (inherited from Python) is to install a few lines in your shell setup to add contextual aliases in a frank project:
- `php` resolves to the PHP container
- `artisan` as well
- `composer` will also directly resolve to the PHP container
- `pnpm`, `npm`, `bun`... same thing

So just run:

```
frank config shell setup >> .zshrc && source ~/.zshrc # or your favourite shell
```

> _Now you can simply copy/paste that command from XYZ documentation!_

### The Go Situation (Mac users especially)

While you get a single binary that enables PHP development without installing them locally, with queue workers and other helpful tool, the only drawback is that you need to have Go installed (so much for not having PHP locally right?).

The good part is that if you're using WSL/Linux, you can download the binary. For Mac users, for now you'll need to install go.

> I'm usually using [Proto](https://moonrepo.dev/proto) to install Go easily - Proto is cool too, as a tool. 
