-- +goose Up
ALTER TABLE user_settings
DROP CONSTRAINT user_settings_video_quality_check;

UPDATE user_settings
SET video_quality = 'original'
WHERE video_quality = 'high';

ALTER TABLE user_settings
ADD CONSTRAINT user_settings_video_quality_check
CHECK (video_quality IN ('low', 'high', 'original'));

-- +goose Down
ALTER TABLE user_settings
DROP CONSTRAINT user_settings_video_quality_check;

UPDATE user_settings
SET video_quality = 'high'
WHERE video_quality = 'original';

ALTER TABLE user_settings
ADD CONSTRAINT user_settings_video_quality_check
CHECK (video_quality IN ('low', 'high'));
