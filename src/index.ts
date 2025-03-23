import { ServerApp } from "./server";
import { environment } from "./environments";

const { port, cacheKey, lyricsUrl, lyricsPort } = environment;

// Создание и запуск экземпляра сервера
new ServerApp(port, cacheKey, lyricsUrl, lyricsPort);
