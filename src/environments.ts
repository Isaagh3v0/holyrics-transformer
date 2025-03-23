import dotenv from 'dotenv'

dotenv.config()

export const environment = {
    port: Number(process.env.PORT),
    cacheKey: process.env.CACHE_KEY,
    lyricsUrl: process.env.HOLYRICS_URL,
    lyricsPort: Number(process.env.HOLYRICS_PORT)
}