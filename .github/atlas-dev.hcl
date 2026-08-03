env "dev" {
  url = getenv("ATLAS_URL")
  migration {
    dir = "file:///migrations"
  }
}
