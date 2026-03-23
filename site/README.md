# Focus landing page

This directory contains the static marketing site for `focus.krabhi.me`.

## Deploy

- Push changes to `main`
- GitHub Actions publishes `site/` plus the root `install.sh` through the Pages workflow
- In GitHub repo settings, set GitHub Pages to use GitHub Actions
- Add the DNS record for `focus.krabhi.me` to point at GitHub Pages

## Files

- `index.html` - landing page
- `styles.css` - visual styling
- `favicon.svg` - browser icon
- `og-image.svg` - social preview image
- `CNAME` - custom domain
