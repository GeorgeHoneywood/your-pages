# your pages

a quick and dirty self-hostable rip-off of netlify/github/sourcehut/cloudflare pages.

it runs on :4444 and accepts a HTTP post like so:

```bash
$ curl -F "mysitename.com=@dist.tar.gz" localhost:4444/upload
> uploaded site mysitename.com
```

once you have your DNS set to CNAME `mysitename.com` to the right IP:

```bash
$ curl mysitename.com:4444
> <html>
>   <p>this is mysitename.com</p>
> </html>
```

under the hood, everything is shoved into an SQLite database and that is it -- not exactly webscale.

if this is to be used, it ought to have some authentication set up for the `/upload` endpoint. otherwise anyone can upload anything and have you host it.
