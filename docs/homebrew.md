# homebrew

the clean setup is:

- app repo: `kacy/chatbubbles`
- tap repo: `kacy/homebrew-chatbubbles`
- formula path in the tap: `Formula/chatbubbles.rb`

the formula should install both `chatbubbles` and `chatbubbles-cli` from the tagged release tarball.

## before you start

for a normal public tap, the release tarballs have to be publicly downloadable.

if `kacy/chatbubbles` stays private, you have two real options:

- make the main repo or just its release artifacts public
- publish the release tarballs somewhere public and point the formula there

the helper script in this repo assumes public github releases from `kacy/chatbubbles`.

## create the tap repo

create a new public repo named:

```text
kacy/homebrew-chatbubbles
```

then add:

```text
Formula/chatbubbles.rb
```

## update flow after each release

1. cut a new release tag in `kacy/chatbubbles`
2. wait for the release workflow to publish:
   - `chatbubbles_<version>_darwin_arm64.tar.gz`
   - `chatbubbles_<version>_darwin_amd64.tar.gz`
   - `checksums.txt`
3. copy the two sha256 values from `checksums.txt`
4. render the new formula from this repo:

```sh
make homebrew-formula ARM64_SHA=<arm64-sha256> AMD64_SHA=<amd64-sha256>
```

or:

```sh
./scripts/render-homebrew-formula.sh "$(cat VERSION)" <arm64-sha256> <amd64-sha256>
```

5. paste the output into `Formula/chatbubbles.rb` in the tap repo
6. run the usual brew checks in the tap repo:

```sh
brew style Formula/chatbubbles.rb
brew audit --strict --online Formula/chatbubbles.rb
brew install --build-from-source Formula/chatbubbles.rb
```

7. commit and push the tap update

## install shape

the formula generated here does a few things on purpose:

- depends on `imsg`
- installs both `chatbubbles` and `chatbubbles-cli`
- keeps tailscale as a manual prerequisite in `caveats`
- leaves macOS permissions in `caveats`, since brew cannot grant them for you

## expected user install

once the tap is live, the install path is:

```sh
brew tap kacy/chatbubbles
brew install kacy/chatbubbles/chatbubbles
```

that gets the binaries onto the machine, but the user still needs:

- tailscale installed and signed in
- Messages signed in
- full disk access and automation permissions granted
