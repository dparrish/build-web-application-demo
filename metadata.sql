CREATE TABLE Metadata (
	Id            STRING(255) NOT NULL,
	UserId        STRING(255) NOT NULL,
	Name          STRING(255) NOT NULL,
	Uploaded      TIMESTAMP NOT NULL,
	MimeType      STRING(32),
	Size          INT64,
) PRIMARY KEY (Id);

CREATE INDEX Metadata_UserId ON Metadata (UserId);

CREATE TABLE Users (
	Id STRING(255) NOT NULL,
	EncryptionKey STRING(MAX),
) PRIMARY KEY (Id);
