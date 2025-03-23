import { createLogger, format, transports } from 'winston';
import 'winston-daily-rotate-file';

const { combine, timestamp, printf, colorize } = format;

// Определение формата для цветного консольного вывода
const consoleFormat = combine(
  colorize(),
  timestamp({ format: 'YYYY-MM-DD HH:mm:ss' }),
  printf(({ timestamp, level, message }) => `${timestamp} [${level}]: ${message}`)
);

// Определение формата для записи в файл
const fileFormat = combine(
  timestamp({ format: 'YYYY-MM-DD HH:mm:ss' }),
  printf(({ timestamp, level, message }) => `${timestamp} [${level}]: ${message}`)
);

// Создание логгера
const logger = createLogger({
  level: 'info',
  format: fileFormat, // Базовый формат для всех транспортов
  transports: [
    // Цветной вывод в консоль
    new transports.Console({
      format: consoleFormat,
    }),
    // Ежедневное сохранение логов в файл activity.log
    new transports.DailyRotateFile({
      filename: 'logs/activity-%DATE%.log',
      datePattern: 'YYYY-MM-DD',
      zippedArchive: true,
      maxSize: '20m',
      maxFiles: '14d',
    }),
  ],
});

export { logger };