import axios from 'axios';
import * as http from 'http';
import { Server } from 'socket.io';
import sqlite3 from 'sqlite3';
import { open, Database } from 'sqlite';
import { logger } from './logger';

export class ServerApp {
  private db: Database | undefined;
  private lastText: string | null = null;
  private lastType: string | null = null;
  private lastHeader: string | null = null;
  private io: Server | undefined;
  private port: number | undefined;
  private cacheKey: string | undefined;
  private lyricsUrl: string | undefined;
  private lyricsPort: number | undefined;
  private connectionStatus: {
    connected: boolean;
    lastError: string | null;
    errorTime: Date | null;
    consecutiveErrors: number;
    reconnecting: boolean;
  } = {
      connected: true,
      lastError: null,
      errorTime: null,
      consecutiveErrors: 0,
      reconnecting: false
    };

  private reconnectTimerActive: boolean = false;

  constructor(
    port: number = 3000,
    cacheKey: string = 'lastText',
    lyricsUrl: string = 'localhost',
    lyricsPort: number = 80
  ) {
    this.initDB()
      .then(() => {
        this.port = port;
        this.cacheKey = cacheKey;
        this.lyricsUrl = lyricsUrl;
        this.lyricsPort = lyricsPort;
        this.startServer();
        this.startTextUpdates();
      })
      .catch((error) => {
        console.error('Ошибка инициализации:', error);
      });
  }

  private async initDB(): Promise<void> {
    this.db = await open({
      filename: './cache.db',
      driver: sqlite3.Database,
    });
    await this.db.exec(`
      CREATE TABLE IF NOT EXISTS cache (
        key TEXT PRIMARY KEY,
        value TEXT
      );
      CREATE TABLE IF NOT EXISTS history (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        text TEXT UNIQUE
      );
      CREATE TABLE IF NOT EXISTS connection_errors (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        error TEXT,
        timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
      );
    `);
    this.lastText = await this.getCachedText();
  }

  private async saveToHistory(text: string): Promise<void> {
    if (this.db) {
      await this.db.run(
        'INSERT INTO history (text) VALUES (?) ON CONFLICT(text) DO NOTHING',
        text
      );
    }
  }

  private async getCachedText(): Promise<string | null> {
    if (this.db) {
      const row = await this.db.get('SELECT value FROM cache WHERE key = ?', this.cacheKey);
      if (row && row.value) return row.value;

      const historyRow = await this.db.get('SELECT text FROM history ORDER BY id DESC LIMIT 1');
      return historyRow ? historyRow.text : null;
    }
    return null;
  }

  private async setCachedText(text: string): Promise<void> {
    if (this.db) {
      await this.db.run(
        'INSERT INTO cache (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?',
        [this.cacheKey, text, text]
      );
      await this.saveToHistory(text);
    }
  }

  private async saveConnectionError(error: string): Promise<void> {
    if (this.db) {
      await this.db.run(
        'INSERT INTO connection_errors (error) VALUES (?)',
        error
      );
    }
  }

  private processBibleText(text: string): string[] {
    let plainText = text.replace(/<[^>]*>/g, '');
    plainText = plainText.replace(/&nbsp;/g, ' ');
    plainText = plainText.replace(/\s+/g, ' ').trim();
    plainText = plainText.replace(/\s*\(.*?\)\s*$/, '');
    const parts = plainText.split(/\d+\.\s*/).filter(p => p.trim() !== '');
    return parts.map(p => p.trim());
  }

  // Обновленный метод для отправки уведомлений о статусе сервера
  private notifyConnectionStatus(status: 'connecting' | 'disconnected' | 'connected'): void {
    if (!this.io) return;

    let message = '';
    let type = 'NOTIFICATION';

    switch (status) {
      case 'connecting':
        message = `Попытка подключения к серверу ${this.lyricsUrl}:${this.lyricsPort}...`;
        break;
      case 'disconnected':
        message = `Потеряно соединение с сервером. ${this.connectionStatus.lastError || ''}`;
        break;
      case 'connected':
        message = `Соединение с сервером успешно установлено.`;
        break;
    }

    // Отправляем уведомление всем клиентам
    this.io.emit('serverState', {
      status,
      connected: this.connectionStatus.connected,
      reconnecting: this.connectionStatus.reconnecting,
      lastError: this.connectionStatus.lastError,
      errorTime: this.connectionStatus.errorTime,
      consecutiveErrors: this.connectionStatus.consecutiveErrors
    });

    // Отправляем информационное сообщение в виде текста
    this.io.emit('notification', { type, message });
  }

  private startServer(): void {
    const server = http.createServer();
    this.io = new Server(server, {
      cors: {
        origin: '*',
        methods: ['GET', 'POST']
      }
    });

    // В обработчике события подключения сокет-клиента:
    this.io.on('connection', (socket) => {
      logger.info(`Клиент подключился: ${socket.id}`);

      // Отправляем текущий статус сервера новому клиенту
      socket.emit('serverState', {
        status: this.connectionStatus.connected
          ? 'connected'
          : (this.connectionStatus.reconnecting ? 'connecting' : 'disconnected'),
        connected: this.connectionStatus.connected,
        reconnecting: this.connectionStatus.reconnecting,
        lastError: this.connectionStatus.lastError,
        errorTime: this.connectionStatus.errorTime,
        consecutiveErrors: this.connectionStatus.consecutiveErrors
      });

      // Если сохранённый текст является сообщением об ошибке, не отправляем его
      if (this.lastText && !this.lastText.startsWith('Ошибка при получении текста')) {
        let content: string | string[] = this.lastText;
        let type = this.lastType || 'TEXT';
        const header = this.lastHeader;
        if (type === 'BIBLE') {
          content = this.processBibleText(this.lastText);
        }
        setTimeout(() => {
          socket.emit('text', { type, header, content });
        }, 50);
      }
    });
    server.listen(this.port!);
  }

  private removeHiddenSpan(text: string): string {
    return text.replace(/<span[^>]*id=["']text-force-update_0["'][^>]*>.*?<\/span>/gi, '');
  }

  // Изменённый метод попытки переподключения, чтобы уведомлять клиентов при каждой попытке:
  private async attemptReconnect(): Promise<boolean> {
    if (!this.connectionStatus.reconnecting) {
      this.connectionStatus.reconnecting = true;
      // Отправляем уведомление о начале попытки подключения
      this.notifyConnectionStatus('connecting');
      logger.info(`Попытка подключения к ${this.lyricsUrl}:${this.lyricsPort}...`);
    }

    try {
      const response = await axios.get(`http://${this.lyricsUrl}:${this.lyricsPort}/view/text.json`, {
        timeout: 5000
      });

      if (response.status === 200) {
        this.connectionStatus.connected = true;
        this.connectionStatus.reconnecting = false;
        this.connectionStatus.consecutiveErrors = 0;
        this.connectionStatus.lastError = null;
        this.reconnectTimerActive = false;

        // Отправляем уведомление об успешном подключении
        this.notifyConnectionStatus('connected');

        logger.info('Соединение с сервером успешно восстановлено');
        return true;
      }

      return false;
    } catch (error) {
      // При неудаче повторяем уведомление о попытке подключения
      this.notifyConnectionStatus('connecting');
      return false;
    }
  }

  private startReconnectionProcess(): void {
    if (this.reconnectTimerActive) {
      return;
    }

    this.reconnectTimerActive = true;

    const attemptConnection = async () => {
      if (!this.connectionStatus.connected) {
        const success = await this.attemptReconnect();

        if (!success) {
          const retryDelay = Math.min(1000 * Math.pow(1.5, Math.min(this.connectionStatus.consecutiveErrors, 10)), 30000);
          setTimeout(attemptConnection, retryDelay);
        } else {
          this.reconnectTimerActive = false;
        }
      } else {
        this.reconnectTimerActive = false;
      }
    };

    setTimeout(attemptConnection, 1000);

    logger.info('Запущен процесс переподключения к серверу');
  }

  private async updateText(): Promise<void> {
    if (this.connectionStatus.reconnecting) {
      return;
    }

    try {
      const response = await axios.get(`http://${this.lyricsUrl}:${this.lyricsPort}/view/text.json`, {
        timeout: 5000
      });

      const data = response.data?.map;
      if (data && typeof data.text === 'string') {
        const cleanedText = this.removeHiddenSpan(data.text);
        if (cleanedText !== this.lastText) {
          this.lastText = cleanedText;
          this.lastType = (data.type || 'TEXT').toUpperCase();
          this.lastHeader = data.header || null;
          await this.setCachedText(cleanedText);

          let content: string | string[] = cleanedText;
          if (this.lastType === 'BIBLE') {
            content = this.processBibleText(cleanedText);
          }
          if (this.io) {
            this.io.emit('text', { type: this.lastType, header: this.lastHeader, content });
          }
          logger.info('Отправлен обновлённый текст');
        }
      }

      if (!this.connectionStatus.connected) {
        this.connectionStatus.connected = true;
        this.connectionStatus.consecutiveErrors = 0;
        this.connectionStatus.lastError = null;

        // Отправляем уведомление о восстановлении соединения
        this.notifyConnectionStatus('connected');

        logger.info('Соединение с сервером восстановлено');
      }
    } catch (error) {
      // Важно: НЕ отправляем ошибку как текстовое сообщение для отображения

      if (this.connectionStatus.connected) {
        const errorMessage = error instanceof Error
          ? error.message
          : 'Неизвестная ошибка при получении текста';

        logger.error('Ошибка при получении текста:', errorMessage);

        await this.saveConnectionError(errorMessage);

        this.connectionStatus.connected = false;
        this.connectionStatus.lastError = errorMessage;
        this.connectionStatus.errorTime = new Date();
        this.connectionStatus.consecutiveErrors = 1;

        // Отправляем уведомление о потере соединения
        this.notifyConnectionStatus('disconnected');

        this.startReconnectionProcess();
      } else {
        this.connectionStatus.consecutiveErrors++;
      }
    }
  }

  private startTextUpdates(): void {
    this.updateText();
    setInterval(() => this.updateText(), 200);
  }
}