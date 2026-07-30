import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import batchImage from './batchImage'
import imageStudio from './imageStudio'
import mediaStudio from './mediaStudio'
import videoStudio from './videoStudio'
import admin from './admin'
import misc from './misc'

export default {
  ...landing,
  ...common,
  ...dashboard,
  ...batchImage,
  ...imageStudio,
  ...mediaStudio,
  ...videoStudio,
  admin,
  ...misc,
}
